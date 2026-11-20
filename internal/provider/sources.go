package provider

// Connections and the sources attached from them (PLAN 42; the API's plane/sources.py, behind the organisation's
// `sources` switch): a connection is a stored login to another system (an S3-compatible bucket today, or a public one
// with no key), owned by its maker and private until shared; a source is a connection attached under a name, read in
// SQL as <source>.<schema>.<table> ("connected catalog" on the account page).

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func orgPath(org, rest string) string { return "/v1/orgs/" + url.PathEscape(org) + rest }

// ---------------------------------------------------------------- lakehousebox_connection

type sourceConnModel struct {
	ID          types.String `tfsdk:"id"`
	Org         types.String `tfsdk:"org"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Endpoint    types.String `tfsdk:"endpoint"`
	Region      types.String `tfsdk:"region"`
	Bucket      types.String `tfsdk:"bucket"`
	Prefix      types.String `tfsdk:"prefix"`
	URLStyle    types.String `tfsdk:"url_style"`
	Anonymous   types.Bool   `tfsdk:"anonymous"`
	AccessKeyID types.String `tfsdk:"access_key_id"`
	SecretKey   types.String `tfsdk:"secret_access_key"`
	UseAccess   types.String `tfsdk:"use_access"`
	OwnerID     types.String `tfsdk:"owner_id"`
	State       types.String `tfsdk:"state"`
}

type sourceConnAPI struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	Endpoint  string `json:"endpoint"`
	Port      int    `json:"port"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	URLStyle  string `json:"url_style"`
	Anonymous bool   `json:"anonymous"`
	UseAccess string `json:"use_access"`
	OwnerID   string `json:"owner_id"`
	State     string `json:"state"`
}

type sourceConnResource struct{ c *Client }

func newSourceConnectionResource() resource.Resource { return &sourceConnResource{} }

func (r *sourceConnResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_connection"
}

func (r *sourceConnResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	keep := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	where := func(d string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Description: d,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}}
	}
	resp.Schema = schema.Schema{
		Description: "A connection to another system that sources read with: an S3-compatible bucket and folder, with a " +
			"key or none (a public bucket). Owned by whoever creates it and private to them until shared (use_access). " +
			"Where it points never changes in place: another bucket or folder is another connection.",
		Attributes: map[string]schema.Attribute{
			"id":        schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"org":       schema.StringAttribute{Required: true, Description: "The organisation's handle or id.", PlanModifiers: replaceStr},
			"name":      schema.StringAttribute{Required: true, Description: "Lower case letters, digits and _.", PlanModifiers: replaceStr},
			"type":      schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("s3"), PlanModifiers: replaceStr},
			"endpoint":  schema.StringAttribute{Required: true, Description: "The store's host name, e.g. s3.eu-central-1.amazonaws.com (https).", PlanModifiers: replaceStr},
			"region":    where("Default us-east-1."),
			"bucket":    schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"prefix":    where("A folder inside the bucket: only keys under it are ever read. Default: the whole bucket."),
			"url_style": where("path (the default) or vhost."),
			"anonymous": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description:   "A public bucket: read with no key (no access_key_id or secret_access_key).",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.RequiresReplace()}},
			"access_key_id": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "The key's id. Write-only at the API (never read back); a new key pair is replaced in place."},
			"secret_access_key": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "The key's secret. Write-only at the API (never read back); kept in state as sensitive values are."},
			"use_access": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("owner"),
				Description: "Who may attach sources from it besides its owner and the admins: owner (nobody) or everyone."},
			"owner_id": schema.StringAttribute{Computed: true, PlanModifiers: keep},
			"state": schema.StringAttribute{Computed: true,
				Description: "active, or stopped: its owner left the organisation (an admin takes it over)."},
		},
	}
}

func (r *sourceConnResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

// sameFolder: the API stores a folder with its trailing / (exports -> exports/)
func sameFolder(a, b string) bool { return strings.TrimSuffix(a, "/") == strings.TrimSuffix(b, "/") }

// sameEndpoint: the API stores a host in lower case without https:// and without :443
func sameEndpoint(a, b string) bool {
	canon := func(s string) string {
		s = strings.TrimSuffix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "https://"), "/")
		return strings.TrimSuffix(s, ":443")
	}
	return canon(a) == canon(b)
}

// from: what the API answers, the configured spelling kept where it means the same (else every refresh would plan a
// replacement for `prefix = "exports"` against the stored `exports/`)
func (m *sourceConnModel) from(a *sourceConnAPI) {
	ep := a.Endpoint
	if a.Port != 0 && a.Port != 443 {
		ep += ":" + strconv.Itoa(a.Port)
	}
	if !sameEndpoint(ep, m.Endpoint.ValueString()) {
		m.Endpoint = types.StringValue(ep)
	}
	if m.Prefix.IsNull() || m.Prefix.IsUnknown() || !sameFolder(a.Prefix, m.Prefix.ValueString()) {
		m.Prefix = types.StringValue(a.Prefix)
	}
	m.Type, m.Region, m.Bucket, m.URLStyle = types.StringValue(a.Type), types.StringValue(a.Region), types.StringValue(a.Bucket), types.StringValue(a.URLStyle)
	m.Anonymous, m.UseAccess = types.BoolValue(a.Anonymous), types.StringValue(a.UseAccess)
	m.OwnerID, m.State = types.StringValue(a.OwnerID), types.StringValue(a.State)
}

func (r *sourceConnResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m sourceConnModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": m.Name.ValueString(), "type": m.Type.ValueString(), "endpoint": m.Endpoint.ValueString(),
		"bucket": m.Bucket.ValueString(), "use_access": m.UseAccess.ValueString()}
	for k, v := range map[string]types.String{"region": m.Region, "prefix": m.Prefix, "url_style": m.URLStyle} {
		if !v.IsNull() && !v.IsUnknown() {
			body[k] = v.ValueString()
		}
	}
	if m.Anonymous.ValueBool() {
		body["anonymous"] = true
	} else {
		body["access_key_id"], body["secret_access_key"] = m.AccessKeyID.ValueString(), m.SecretKey.ValueString()
	}
	var out struct {
		Connection sourceConnAPI `json:"connection"`
	}
	if err := r.c.Do(ctx, "POST", orgPath(m.Org.ValueString(), "/connections"), nil, body, &out); err != nil {
		apiErr(&resp.Diagnostics, "create connection "+m.Name.ValueString(), err)
		return
	}
	m.ID = types.StringValue(m.Org.ValueString() + "|" + m.Name.ValueString())
	m.from(&out.Connection)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sourceConnResource) get(ctx context.Context, org, name string) (*sourceConnAPI, error) {
	var out struct {
		Connection sourceConnAPI `json:"connection"`
	}
	err := r.c.Do(ctx, "GET", orgPath(org, "/connections/"+url.PathEscape(name)), nil, nil, &out)
	return &out.Connection, err
}

func (r *sourceConnResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m sourceConnModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	a, err := r.get(ctx, m.Org.ValueString(), m.Name.ValueString())
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read connection", err)
		return
	}
	m.from(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update: the key pair (PUT …/secret, the connection kept, so its sources never stop) and use_access; every other
// attribute replaces the connection.
func (r *sourceConnResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior sourceConnModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org, name := m.Org.ValueString(), m.Name.ValueString()
	if !m.Anonymous.ValueBool() && (!m.AccessKeyID.Equal(prior.AccessKeyID) || !m.SecretKey.Equal(prior.SecretKey)) {
		if err := r.c.Do(ctx, "PUT", orgPath(org, "/connections/"+url.PathEscape(name)+"/secret"), nil,
			map[string]string{"access_key_id": m.AccessKeyID.ValueString(), "secret_access_key": m.SecretKey.ValueString()}, nil); err != nil {
			apiErr(&resp.Diagnostics, "replace the key of connection "+name, err)
			return
		}
	}
	if !m.UseAccess.Equal(prior.UseAccess) {
		if err := r.c.Do(ctx, "PUT", orgPath(org, "/connections/"+url.PathEscape(name)+"/use-access"), nil,
			map[string]string{"use_access": m.UseAccess.ValueString()}, nil); err != nil {
			apiErr(&resp.Diagnostics, "change who may use connection "+name, err)
			return
		}
	}
	a, err := r.get(ctx, org, name)
	if err != nil {
		apiErr(&resp.Diagnostics, "read connection", err)
		return
	}
	m.ID = prior.ID
	m.from(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// `tofu import lakehousebox_connection.x '<org>|<name>'`. The key is never readable: the first apply after an import
// sends the configured key pair in place (PUT …/secret); nothing is deleted, so no source stops.
func (r *sourceConnResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	org, name, ok := splitID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("LakehouseBox: import connection", "the id is <org>|<name>, got "+req.ID)
		return
	}
	a, err := r.get(ctx, org, name)
	if err != nil {
		apiErr(&resp.Diagnostics, "import connection "+req.ID, err)
		return
	}
	m := sourceConnModel{ID: types.StringValue(req.ID), Org: types.StringValue(org), Name: types.StringValue(name),
		Endpoint: types.StringNull(), Prefix: types.StringNull(), AccessKeyID: types.StringNull(), SecretKey: types.StringNull()}
	m.from(a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sourceConnResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m sourceConnModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	err := r.c.Do(ctx, "DELETE", orgPath(m.Org.ValueString(), "/connections/"+url.PathEscape(m.Name.ValueString())), nil, nil, nil)
	if err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "delete connection (a connection a source still uses is refused: detach the source first)", err)
	}
}

// ---------------------------------------------------------------- lakehousebox_source

type sourceModel struct {
	ID         types.String `tfsdk:"id"`
	Org        types.String `tfsdk:"org"`
	Name       types.String `tfsdk:"name"`
	Connection types.String `tfsdk:"connection_name"`
	OrgAccess  types.String `tfsdk:"org_access"`
	Tables     types.List   `tfsdk:"tables"`
}

type sourceResource struct{ c *Client }

func newSourceResource() resource.Resource { return &sourceResource{} }

func (r *sourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_source"
}

func (r *sourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A connection attached under a name (a connected catalog): SQL reads its tables where they live as " +
			"<name>.<schema>.<table>. Created, it looks at its files once; destroyed, it is detached (the data is never touched).",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"org":             schema.StringAttribute{Required: true, Description: "The organisation's handle or id.", PlanModifiers: replaceStr},
			"name":            schema.StringAttribute{Required: true, Description: "How SQL names it; not a catalog's name.", PlanModifiers: replaceStr},
			"connection_name": schema.StringAttribute{Required: true, Description: "The connection's name.", PlanModifiers: replaceStr},
			"org_access": schema.StringAttribute{Optional: true, Computed: true, Default: stringdefault.StaticString("none"),
				Description: "none (Restricted: admins and those granted it) or read (every member of the organisation)."},
			"tables": schema.ListAttribute{Computed: true, ElementType: types.StringType,
				Description:   "The tables found in its files, as SQL names them (<name>.<schema>.<table>), at the last look.",
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *sourceResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *sourceResource) tables(ctx context.Context, org, name string) (types.List, error) {
	var out struct {
		Tables []struct {
			SQLName string `json:"sql_name"`
		} `json:"tables"`
	}
	if err := r.c.Do(ctx, "GET", orgPath(org, "/sources/"+url.PathEscape(name)+"/tables"), nil, nil, &out); err != nil {
		return types.ListNull(types.StringType), err
	}
	names := make([]string, 0, len(out.Tables))
	for _, t := range out.Tables {
		names = append(names, t.SQLName)
	}
	l, _ := types.ListValueFrom(context.Background(), types.StringType, names)
	return l, nil
}

func (r *sourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m sourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org, name := m.Org.ValueString(), m.Name.ValueString()
	if err := r.c.Do(ctx, "POST", orgPath(org, "/sources"), nil, map[string]string{"name": name,
		"connection": m.Connection.ValueString(), "org_access": m.OrgAccess.ValueString()}, nil); err != nil {
		apiErr(&resp.Diagnostics, "attach source "+name, err)
		return
	}
	m.ID = types.StringValue(org + "|" + name)
	// the first look at its files: a source that cannot be listed yet still exists (a warning, not a failed apply)
	if err := r.c.Do(ctx, "POST", orgPath(org, "/sources/"+url.PathEscape(name)+"/refresh"), nil, map[string]string{}, nil); err != nil {
		resp.Diagnostics.AddWarning("LakehouseBox: look at the files of source "+name,
			err.Error()+" (the source is attached; `lhbox source refresh "+name+"` looks again)")
	}
	t, err := r.tables(ctx, org, name)
	if err != nil {
		t = types.ListNull(types.StringType)
	}
	m.Tables = t
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m sourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var out struct {
		Source struct {
			Connection string `json:"connection"`
			OrgAccess  string `json:"org_access"`
		} `json:"source"`
	}
	err := r.c.Do(ctx, "GET", orgPath(m.Org.ValueString(), "/sources/"+url.PathEscape(m.Name.ValueString())), nil, nil, &out)
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read source", err)
		return
	}
	m.Connection, m.OrgAccess = types.StringValue(out.Source.Connection), types.StringValue(out.Source.OrgAccess)
	if t, err := r.tables(ctx, m.Org.ValueString(), m.Name.ValueString()); err == nil {
		m.Tables = t
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update: only org_access changes in place.
func (r *sourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior sourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !m.OrgAccess.Equal(prior.OrgAccess) {
		if err := r.c.Do(ctx, "PUT", orgPath(m.Org.ValueString(), "/sources/"+url.PathEscape(m.Name.ValueString())+"/org-access"),
			nil, map[string]string{"org_access": m.OrgAccess.ValueString()}, nil); err != nil {
			apiErr(&resp.Diagnostics, "change who reads source "+m.Name.ValueString(), err)
			return
		}
	}
	m.ID, m.Tables = prior.ID, prior.Tables
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// `tofu import lakehousebox_source.x '<org>|<name>'`
func (r *sourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	org, name, ok := splitID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("LakehouseBox: import source", "the id is <org>|<name>, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &sourceModel{ID: types.StringValue(req.ID), Org: types.StringValue(org),
		Name: types.StringValue(name), Connection: types.StringUnknown(), OrgAccess: types.StringUnknown(),
		Tables: types.ListNull(types.StringType)})...)
}

func (r *sourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m sourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	err := r.c.Do(ctx, "DELETE", orgPath(m.Org.ValueString(), "/sources/"+url.PathEscape(m.Name.ValueString())), nil, nil, nil)
	if err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "detach source", err)
	}
}
