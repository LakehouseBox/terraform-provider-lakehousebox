package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	dschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type catalogAPI struct {
	WarehouseID          string `json:"warehouse_id"`
	Name                 string `json:"name"`
	ProjectID            string `json:"project_id"`
	DefaultFormatVersion *int64 `json:"default_format_version"`
	Public               bool   `json:"public"`
	Bucket               string `json:"bucket"`
}

type catalogModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	ProjectID            types.String `tfsdk:"project_id"`
	DefaultFormatVersion types.Int64  `tfsdk:"default_format_version"`
	Public               types.Bool   `tfsdk:"public"`
	DeletionProtection   types.Bool   `tfsdk:"deletion_protection"`
	Bucket               types.String `tfsdk:"bucket"`
}

type catalogResource struct{ c *Client }

func newCatalogResource() resource.Resource { return &catalogResource{} }

func (r *catalogResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog"
}

func (r *catalogResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A catalog (an Iceberg REST catalog and its storage). Destroying one starts its grace period " +
			"(7 days, restorable) and only with deletion_protection = false.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The catalog id (warehouse_id).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name": schema.StringAttribute{Required: true, Description: "^[a-z][a-z0-9_]{0,62}$; fixed once made."},
			"project_id": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "The project it is made in (default: the organisation's first); a catalog never moves.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"default_format_version": schema.Int64Attribute{Optional: true, Computed: true,
				Description: "2 or 3: the format version LakehouseBox's own create paths give a new table."},
			"public": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Readable by anyone (every data file and retained metadata): admins only."},
			"deletion_protection": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				Description: "While true (the default), destroy refuses: set false first."},
			"bucket": schema.StringAttribute{Computed: true, Description: "The catalog's table bucket.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *catalogResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *catalogResource) read(ctx context.Context, ref string) (*catalogAPI, error) {
	var out catalogAPI
	err := r.c.Do(ctx, "GET", "/v1/catalogs/"+ref, nil, nil, &out)
	return &out, err
}

func (m *catalogModel) from(a *catalogAPI) {
	m.ID, m.Name, m.ProjectID = types.StringValue(a.WarehouseID), types.StringValue(a.Name), types.StringValue(a.ProjectID)
	m.Public, m.Bucket = types.BoolValue(a.Public), types.StringValue(a.Bucket)
	if a.DefaultFormatVersion != nil {
		m.DefaultFormatVersion = types.Int64Value(*a.DefaultFormatVersion)
	}
}

func (r *catalogResource) setPublic(ctx context.Context, a *catalogAPI, public bool) error {
	if public == a.Public {
		return nil
	}
	if public {
		return r.c.Do(ctx, "POST", "/v1/warehouses/"+a.WarehouseID+"/public", nil, map[string]string{"confirm": a.Name}, nil)
	}
	return r.c.Do(ctx, "DELETE", "/v1/warehouses/"+a.WarehouseID+"/public", nil, nil, nil)
}

func (r *catalogResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m catalogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": m.Name.ValueString()}
	if !m.ProjectID.IsUnknown() && !m.ProjectID.IsNull() {
		body["project_id"] = m.ProjectID.ValueString()
	}
	if !m.DefaultFormatVersion.IsUnknown() && !m.DefaultFormatVersion.IsNull() {
		body["default_format_version"] = m.DefaultFormatVersion.ValueInt64()
	}
	var made catalogAPI
	if err := r.c.Do(ctx, "POST", "/v1/warehouses", nil, body, &made); err != nil {
		apiErr(&resp.Diagnostics, "create catalog", err)
		return
	}
	// made: into state now, whatever follows, so a later failure never leaves it orphaned or tainted (a protected
	// catalog that cannot be replaced; high review of #3032). What follows only warns; the next plan shows what is left
	wantPublic := m.Public.ValueBool()
	m.from(&made)
	if wantPublic {
		if err := r.setPublic(ctx, &made, true); err != nil {
			resp.Diagnostics.AddWarning("LakehouseBox: the catalog was made but not published", err.Error())
		}
	}
	if a, err := r.read(ctx, made.WarehouseID); err == nil {
		m.from(a)
	} else {
		resp.Diagnostics.AddWarning("LakehouseBox: read the new catalog", err.Error())
	}
	if m.DefaultFormatVersion.IsUnknown() {
		m.DefaultFormatVersion = types.Int64Null()
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// ModifyPlan refuses what cannot be done in place and must not be done by replacement: a catalog's name and its
// project are fixed (a deleted catalog keeps its name and bucket reserved for its 7-day grace period, so a destroy and
// create would leave no catalog at all; high review of #3032).
func (r *catalogResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state catalogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if !plan.Name.Equal(state.Name) {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "A catalog is not renamed",
			"Catalog "+state.Name.ValueString()+" keeps its name. Declare another lakehousebox_catalog for a new one.")
	}
	if !plan.ProjectID.IsUnknown() && !plan.ProjectID.Equal(state.ProjectID) {
		resp.Diagnostics.AddAttributeError(path.Root("project_id"), "A catalog does not move between projects",
			"Catalog "+state.Name.ValueString()+" stays in project "+state.ProjectID.ValueString()+".")
	}
}

func (r *catalogResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m catalogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	a, err := r.read(ctx, m.ID.ValueString())
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read catalog", err)
		return
	}
	m.from(a)
	if m.DeletionProtection.IsNull() {
		m.DeletionProtection = types.BoolValue(true)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *catalogResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior catalogModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	a, err := r.read(ctx, prior.ID.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "read catalog", err)
		return
	}
	if !m.DefaultFormatVersion.IsUnknown() && !m.DefaultFormatVersion.IsNull() &&
		(a.DefaultFormatVersion == nil || *a.DefaultFormatVersion != m.DefaultFormatVersion.ValueInt64()) {
		if err := r.c.Do(ctx, "PATCH", "/v1/warehouses/"+a.WarehouseID, nil,
			map[string]any{"default_format_version": m.DefaultFormatVersion.ValueInt64()}, nil); err != nil {
			apiErr(&resp.Diagnostics, "set default_format_version", err)
			return
		}
	}
	if err := r.setPublic(ctx, a, m.Public.ValueBool()); err != nil {
		apiErr(&resp.Diagnostics, "change public", err)
		return
	}
	if a, err = r.read(ctx, a.WarehouseID); err == nil {
		m.from(a)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *catalogResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m catalogModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if err := deleteAllowed(m); err != nil {
		resp.Diagnostics.AddError("Catalog is protected", err.Error())
		return
	}
	if err := r.c.Do(ctx, "DELETE", "/v1/warehouses/"+m.ID.ValueString(), nil,
		map[string]string{"confirm": m.Name.ValueString()}, nil); err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "delete catalog", err)
	}
}

// deleteAllowed: a catalog is deleted only with deletion_protection explicitly false (null, as in a state written before
// the attribute, is protected)
func deleteAllowed(m catalogModel) error {
	if m.DeletionProtection.IsNull() || m.DeletionProtection.IsUnknown() || m.DeletionProtection.ValueBool() {
		return fmt.Errorf("catalog %s has deletion_protection = true: set it to false and apply first. Deleting starts a "+
			"7-day grace period (restorable), then its tables and files are purged", m.Name.ValueString())
	}
	return nil
}

func (r *catalogResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	a, err := r.read(ctx, req.ID)
	if err != nil {
		apiErr(&resp.Diagnostics, "import catalog "+req.ID, err)
		return
	}
	var m catalogModel
	m.from(a)
	m.DeletionProtection = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// ---------------------------------------------------------------- data source

type catalogDataSource struct{ c *Client }

func newCatalogDataSource() datasource.DataSource { return &catalogDataSource{} }

func (d *catalogDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_catalog"
}

type catalogDataModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	ProjectID            types.String `tfsdk:"project_id"`
	DefaultFormatVersion types.Int64  `tfsdk:"default_format_version"`
	Public               types.Bool   `tfsdk:"public"`
	Bucket               types.String `tfsdk:"bucket"`
}

func (d *catalogDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = dschema.Schema{
		Description: "A catalog the key can read, by name.",
		Attributes: map[string]dschema.Attribute{
			"name":                   dschema.StringAttribute{Required: true},
			"id":                     dschema.StringAttribute{Computed: true},
			"project_id":             dschema.StringAttribute{Computed: true},
			"default_format_version": dschema.Int64Attribute{Computed: true},
			"public":                 dschema.BoolAttribute{Computed: true},
			"bucket":                 dschema.StringAttribute{Computed: true},
		},
	}
}

func (d *catalogDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if c, ok := req.ProviderData.(*Client); ok {
		d.c = c
	}
}

func (d *catalogDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m catalogDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	var a catalogAPI
	if err := d.c.Do(ctx, "GET", "/v1/catalogs/"+m.Name.ValueString(), nil, nil, &a); err != nil {
		apiErr(&resp.Diagnostics, "read catalog", err)
		return
	}
	m.ID, m.ProjectID, m.Public, m.Bucket = types.StringValue(a.WarehouseID), types.StringValue(a.ProjectID),
		types.BoolValue(a.Public), types.StringValue(a.Bucket)
	if a.DefaultFormatVersion != nil {
		m.DefaultFormatVersion = types.Int64Value(*a.DefaultFormatVersion)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
