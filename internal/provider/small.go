package provider

import (
	"context"
	"net/url"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var replaceStr = []planmodifier.String{stringplanmodifier.RequiresReplace()}

// ---------------------------------------------------------------- POST prefix

type postPrefixModel struct {
	ID        types.String `tfsdk:"id"`
	Catalog   types.String `tfsdk:"catalog"`
	URLPrefix types.String `tfsdk:"url_prefix"`
}

type postPrefixResource struct{ c *Client }

func newPostPrefixResource() resource.Resource { return &postPrefixResource{} }

func (r *postPrefixResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sql_post_prefix"
}

func (r *postPrefixResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A URL prefix the catalog's scheduled SQL may POST to (http_post).",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"catalog":    schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"url_prefix": schema.StringAttribute{Required: true, Description: "https://…/ ending in /.", PlanModifiers: replaceStr},
		},
	}
}

func (r *postPrefixResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *postPrefixResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m postPrefixModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.Do(ctx, "POST", "/v1/sql/post-prefixes", nil,
		map[string]string{"warehouse": m.Catalog.ValueString(), "url_prefix": m.URLPrefix.ValueString()}, nil); err != nil {
		apiErr(&resp.Diagnostics, "declare POST prefix", err)
		return
	}
	m.ID = types.StringValue(m.Catalog.ValueString() + "|" + m.URLPrefix.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *postPrefixResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m postPrefixModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var out struct {
		PostPrefixes []struct {
			URLPrefix string `json:"url_prefix"`
		} `json:"post_prefixes"`
	}
	if err := r.c.Do(ctx, "GET", "/v1/sql/post-prefixes", url.Values{"warehouse": {m.Catalog.ValueString()}}, nil, &out); err != nil {
		if NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "list POST prefixes", err)
		return
	}
	for _, p := range out.PostPrefixes {
		if samePrefix(p.URLPrefix, m.URLPrefix.ValueString()) {
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *postPrefixResource) Update(context.Context, resource.UpdateRequest, *resource.UpdateResponse) {
}

// `tofu import lakehousebox_sql_post_prefix.x '<catalog>|<url_prefix>'`; Read then checks the prefix is declared.
func (r *postPrefixResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	cat, prefix, ok := splitID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("LakehouseBox: import POST prefix", "the id is <catalog>|<url_prefix>, got "+req.ID)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &postPrefixModel{ID: types.StringValue(req.ID),
		Catalog: types.StringValue(cat), URLPrefix: types.StringValue(prefix)})...)
}

func (r *postPrefixResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m postPrefixModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	err := r.c.Do(ctx, "DELETE", "/v1/sql/post-prefixes", url.Values{"warehouse": {m.Catalog.ValueString()},
		"url_prefix": {m.URLPrefix.ValueString()}}, nil, nil)
	if err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "remove POST prefix", err)
	}
}

// ---------------------------------------------------------------- connection

type connectionModel struct {
	ID          types.String `tfsdk:"id"`
	Catalog     types.String `tfsdk:"catalog"`
	Name        types.String `tfsdk:"name"`
	Integration types.String `tfsdk:"integration"`
	URLPrefix   types.String `tfsdk:"url_prefix"`
	Kind        types.String `tfsdk:"kind"`
	HeaderName  types.String `tfsdk:"header_name"`
	Value       types.String `tfsdk:"value"`
}

type connectionResource struct{ c *Client }

func newConnectionResource() resource.Resource { return &connectionResource{} }

func (r *connectionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sql_connection"
}

func (r *connectionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	opt := func(d string) schema.StringAttribute {
		return schema.StringAttribute{Optional: true, Computed: true, Description: d,
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}}
	}
	resp.Schema = schema.Schema{
		Description: "A write-only credential for the catalog's scheduled SQL, bound to one URL prefix: an organisation " +
			"web key (integration), or a value (bearer, header, basic). Any change makes a new one.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"catalog":     schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"name":        schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"integration": schema.StringAttribute{Optional: true, Description: "An organisation web key's name.", PlanModifiers: replaceStr},
			"url_prefix":  opt("https://…/ (from the web key with integration)."),
			"kind":        opt("bearer, header or basic."),
			"header_name": opt("The header, with kind header."),
			"value": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "The secret (never readable back). Kept in state as Terraform keeps sensitive values: prefer integration."},
		},
	}
}

func (r *connectionResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

type connAPI struct {
	URLPrefix     string `json:"url_prefix"`
	Kind          string `json:"kind"`
	HeaderName    string `json:"header_name"`
	IntegrationID string `json:"integration_id"`
}

func (m *connectionModel) from(a *connAPI) {
	if !samePrefix(a.URLPrefix, m.URLPrefix.ValueString()) {
		m.URLPrefix = types.StringValue(a.URLPrefix) // the configured spelling stays while it means the same
	}
	m.Kind = types.StringValue(a.Kind)
	if a.HeaderName != "" {
		m.HeaderName = types.StringValue(a.HeaderName)
	} else {
		m.HeaderName = types.StringNull()
	}
}

func (r *connectionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m connectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"warehouse": m.Catalog.ValueString(), "name": m.Name.ValueString()}
	if !m.Integration.IsNull() {
		body["integration"] = m.Integration.ValueString()
	} else {
		body["url_prefix"], body["kind"], body["value"] = m.URLPrefix.ValueString(), m.Kind.ValueString(), m.Value.ValueString()
		if !m.HeaderName.IsNull() && !m.HeaderName.IsUnknown() {
			body["header_name"] = m.HeaderName.ValueString()
		}
	}
	var a connAPI
	if err := r.c.Do(ctx, "POST", "/v1/sql/connections", nil, body, &a); err != nil {
		apiErr(&resp.Diagnostics, "create connection "+m.Name.ValueString(), err)
		return
	}
	m.ID = types.StringValue(m.Catalog.ValueString() + "|" + m.Name.ValueString())
	m.from(&a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *connectionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var a connAPI
	err := r.c.Do(ctx, "GET", "/v1/sql/connections/"+url.PathEscape(m.Name.ValueString()),
		url.Values{"warehouse": {m.Catalog.ValueString()}}, nil, &a)
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read connection", err)
		return
	}
	m.from(&a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// Update: only `value` changes in place (every other attribute forces a new connection); a new value is sent with PUT,
// the key rotated without a delete, so the sinks that use the connection are never paused.
func (r *connectionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior connectionModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// a value first written into state (after an import) is the live one as far as OpenTofu knows: send it, so the
	// configured secret and the live one agree
	if !m.Value.IsNull() && !m.Value.Equal(prior.Value) && m.Integration.IsNull() {
		body := map[string]any{"value": m.Value.ValueString()}
		if !m.HeaderName.IsNull() && !m.HeaderName.IsUnknown() {
			body["header_name"] = m.HeaderName.ValueString()
		}
		var a connAPI
		if err := r.c.Do(ctx, "PUT", "/v1/sql/connections/"+url.PathEscape(m.Name.ValueString()),
			url.Values{"warehouse": {m.Catalog.ValueString()}}, body, &a); err != nil {
			apiErr(&resp.Diagnostics, "replace the value of connection "+m.Name.ValueString(), err)
			return
		}
		m.from(&a)
	}
	m.ID = prior.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// `tofu import lakehousebox_sql_connection.x '<catalog>|<name>'`. The value is never readable: the first apply after an
// import sends the configured one (PUT, in place). A web-key connection reads back its key's name.
func (r *connectionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	cat, name, ok := splitID(req.ID)
	if !ok {
		resp.Diagnostics.AddError("LakehouseBox: import connection", "the id is <catalog>|<name>, got "+req.ID)
		return
	}
	var a connAPI
	if err := r.c.Do(ctx, "GET", "/v1/sql/connections/"+url.PathEscape(name), url.Values{"warehouse": {cat}}, nil, &a); err != nil {
		apiErr(&resp.Diagnostics, "import connection "+req.ID, err)
		return
	}
	m := connectionModel{ID: types.StringValue(req.ID), Catalog: types.StringValue(cat), Name: types.StringValue(name),
		Integration: types.StringNull(), URLPrefix: types.StringValue(a.URLPrefix), Value: types.StringNull()}
	if a.IntegrationID != "" {
		m.Integration = types.StringValue(r.integrationName(ctx, a.IntegrationID))
	}
	m.from(&a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// integrationName: a web key's name from its id (what a configuration writes), else the id (the API takes either)
func (r *connectionResource) integrationName(ctx context.Context, id string) string {
	var out struct {
		Integrations []struct {
			IntegrationID string `json:"integration_id"`
			Name          string `json:"name"`
		} `json:"integrations"`
	}
	if r.c.Do(ctx, "GET", "/v1/integrations", nil, nil, &out) == nil {
		for _, i := range out.Integrations {
			if i.IntegrationID == id {
				return i.Name
			}
		}
	}
	return id
}

func (r *connectionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m connectionModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	err := r.c.Do(ctx, "DELETE", "/v1/sql/connections/"+url.PathEscape(m.Name.ValueString()),
		url.Values{"warehouse": {m.Catalog.ValueString()}}, nil, nil)
	if err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "delete connection", err)
	}
}

// ---------------------------------------------------------------- AGENTS.md

type agentsMDModel struct {
	ID      types.String `tfsdk:"id"`
	Catalog types.String `tfsdk:"catalog"`
	Text    types.String `tfsdk:"text"`
}

type agentsMDResource struct{ c *Client }

func newAgentsMDResource() resource.Resource { return &agentsMDResource{} }

func (r *agentsMDResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog_agents_md"
}

func (r *agentsMDResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The catalog's AGENTS.md: what its tables mean, for the agents that read them (64 KB at most).",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"catalog": schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"text":    schema.StringAttribute{Required: true, Description: "file(\"docs/AGENTS.md\")."},
		},
	}
}

func (r *agentsMDResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *agentsMDResource) put(ctx context.Context, m *agentsMDModel) error {
	return r.c.Do(ctx, "PUT", "/v1/catalogs/"+m.Catalog.ValueString()+"/agents-md", nil, map[string]string{"text": m.Text.ValueString()}, nil)
}

func (r *agentsMDResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m agentsMDModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if err := r.put(ctx, &m); err != nil {
		apiErr(&resp.Diagnostics, "write AGENTS.md", err)
		return
	}
	m.ID = m.Catalog
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *agentsMDResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m agentsMDModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var out struct {
		AgentsMD struct {
			Present bool   `json:"present"`
			Text    string `json:"text"`
		} `json:"agents_md"`
	}
	if err := r.c.Do(ctx, "GET", "/v1/catalogs/"+m.Catalog.ValueString()+"/agents-md", nil, nil, &out); err != nil {
		if NotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "read AGENTS.md", err)
		return
	}
	if !out.AgentsMD.Present {
		resp.State.RemoveResource(ctx)
		return
	}
	m.Text = types.StringValue(out.AgentsMD.Text)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *agentsMDResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m agentsMDModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if err := r.put(ctx, &m); err != nil {
		apiErr(&resp.Diagnostics, "write AGENTS.md", err)
		return
	}
	m.ID = m.Catalog
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// `tofu import lakehousebox_catalog_agents_md.x <catalog>`; Read then fills the text.
func (r *agentsMDResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, &agentsMDModel{ID: types.StringValue(req.ID),
		Catalog: types.StringValue(req.ID), Text: types.StringNull()})...)
}

func (r *agentsMDResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m agentsMDModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if err := r.c.Do(ctx, "DELETE", "/v1/catalogs/"+m.Catalog.ValueString()+"/agents-md", nil, nil, nil); err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "remove AGENTS.md", err)
	}
}

// ---------------------------------------------------------------- a table's description

type tableDescModel struct {
	ID          types.String `tfsdk:"id"`
	Catalog     types.String `tfsdk:"catalog"`
	Table       types.String `tfsdk:"table"`
	Description types.String `tfsdk:"description"`
	Columns     types.Map    `tfsdk:"columns"`
}

type tableDescResource struct{ c *Client }

func newTableDescriptionResource() resource.Resource { return &tableDescResource{} }

func (r *tableDescResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_table_description"
}

func (r *tableDescResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A table's description and its columns' (what `lhbox table document` writes). The table itself " +
			"comes from migrations; destroying this removes the descriptions only.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"catalog":     schema.StringAttribute{Required: true, PlanModifiers: replaceStr},
			"table":       schema.StringAttribute{Required: true, Description: "namespace.table.", PlanModifiers: replaceStr},
			"description": schema.StringAttribute{Optional: true},
			"columns":     schema.MapAttribute{Optional: true, ElementType: types.StringType, Description: "column -> description."},
		},
	}
}

func (r *tableDescResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *tableDescResource) patch(ctx context.Context, m *tableDescModel, prior *tableDescModel) error {
	ns, name, _ := strings.Cut(m.Table.ValueString(), ".")
	body := map[string]any{"catalog_id": m.Catalog.ValueString(), "namespace": ns, "name": name}
	// the description only when this resource says one, or removes the one it set (never one it does not manage)
	if !m.Description.IsNull() {
		body["doc"] = m.Description.ValueString()
	} else if prior != nil && !prior.Description.IsNull() {
		body["doc"] = nil
	}
	cols := map[string]any{}
	if !m.Columns.IsNull() {
		var c map[string]string
		m.Columns.ElementsAs(ctx, &c, false)
		for k, v := range c {
			cols[k] = v
		}
	}
	if prior != nil && !prior.Columns.IsNull() {
		var old map[string]string
		prior.Columns.ElementsAs(ctx, &old, false)
		for k := range old {
			if _, ok := cols[k]; !ok {
				cols[k] = nil // a column no longer described here
			}
		}
	}
	if len(cols) > 0 {
		body["column_docs"] = cols
	}
	return r.c.Do(ctx, "PATCH", "/v1/table", nil, body, nil)
}

func (r *tableDescResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m tableDescModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if err := r.patch(ctx, &m, nil); err != nil {
		apiErr(&resp.Diagnostics, "describe "+m.Table.ValueString(), err)
		return
	}
	m.ID = types.StringValue(m.Catalog.ValueString() + "|" + m.Table.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tableDescResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m tableDescModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var t tableDocs
	err := r.get(ctx, m.Catalog.ValueString(), m.Table.ValueString(), &t)
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read "+m.Table.ValueString(), err)
		return
	}
	// only what this resource sets: a description it does not manage is never read into state (nor removed later)
	if !m.Description.IsNull() {
		m.Description = types.StringValue(t.Doc)
	}
	if !m.Columns.IsNull() {
		var want map[string]string
		m.Columns.ElementsAs(ctx, &want, false)
		live := map[string]string{}
		for _, c := range t.Schema {
			if _, ok := want[c.Name]; ok && c.Doc != "" {
				live[c.Name] = c.Doc
			}
		}
		v, _ := types.MapValueFrom(ctx, types.StringType, live)
		m.Columns = v
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tableDescResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior tableDescModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if err := r.patch(ctx, &m, &prior); err != nil {
		apiErr(&resp.Diagnostics, "describe "+m.Table.ValueString(), err)
		return
	}
	m.ID = prior.ID
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *tableDescResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m tableDescModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	empty := tableDescModel{Catalog: m.Catalog, Table: m.Table, Description: types.StringNull(), Columns: types.MapNull(types.StringType)}
	if err := r.patch(ctx, &empty, &m); err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "remove the descriptions of "+m.Table.ValueString(), err)
	}
}

// `tofu import lakehousebox_table_description.x '<catalog>|<namespace>.<table>'`: the table's live description and
// every column description it has come into state, so a configuration that declares them plans no change. A
// description the configuration does not name then shows in the plan as one to remove: leave it out of state by
// naming it, or apply to remove it.
func (r *tableDescResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	cat, table, ok := splitID(req.ID)
	if ns, name, dot := strings.Cut(table, "."); !ok || !dot || ns == "" || name == "" {
		resp.Diagnostics.AddError("LakehouseBox: import table description", "the id is <catalog>|<namespace>.<table>, got "+req.ID)
		return
	}
	var t tableDocs
	if err := r.get(ctx, cat, table, &t); err != nil {
		apiErr(&resp.Diagnostics, "import "+req.ID, err)
		return
	}
	m := tableDescModel{ID: types.StringValue(req.ID), Catalog: types.StringValue(cat), Table: types.StringValue(table),
		Description: types.StringNull(), Columns: types.MapNull(types.StringType)}
	if t.Doc != "" {
		m.Description = types.StringValue(t.Doc)
	}
	live := map[string]string{}
	for _, c := range t.Schema {
		if c.Doc != "" {
			live[c.Name] = c.Doc
		}
	}
	if len(live) > 0 {
		v, d := types.MapValueFrom(ctx, types.StringType, live)
		resp.Diagnostics.Append(d...)
		m.Columns = v
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

type tableDocs struct {
	Doc    string `json:"doc"`
	Schema []struct {
		Name string `json:"name"`
		Doc  string `json:"doc"`
	} `json:"schema"`
}

func (r *tableDescResource) get(ctx context.Context, cat, table string, t *tableDocs) error {
	ns, name, _ := strings.Cut(table, ".")
	return r.c.Do(ctx, "GET", "/v1/table", url.Values{"warehouse": {cat}, "namespace": {ns}, "name": {name}}, nil, t)
}

// splitID: "<catalog>|<rest>" (a catalog name never holds a |)
func splitID(id string) (string, string, bool) {
	a, b, ok := strings.Cut(id, "|")
	return a, b, ok && a != "" && b != ""
}

// samePrefix: the API stores a prefix in its canonical form (lower-case host, no :443), so a configured
// https://Host:443/x/ is the same prefix as https://host/x/ (high review of #3032)
func samePrefix(a, b string) bool { return canonPrefix(a) == canonPrefix(b) }

func canonPrefix(p string) string {
	u, err := url.Parse(p)
	if err != nil || u.Host == "" {
		return p
	}
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" && !(u.Scheme == "https" && port == "443") {
		host += ":" + port
	}
	return strings.ToLower(u.Scheme) + "://" + host + u.EscapedPath()
}
