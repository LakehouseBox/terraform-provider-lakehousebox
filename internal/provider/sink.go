package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type sinkAPI struct {
	SinkID        string   `json:"sink_id"`
	WarehouseName string   `json:"warehouse_name"`
	Name          string   `json:"name"`
	Table         string   `json:"table"`
	Kind          string   `json:"kind"`
	Key           []string `json:"key"`
	SQL           *struct {
		Query         string `json:"query"`
		EverySeconds  int64  `json:"every_seconds"`
		OffsetSeconds int64  `json:"offset_seconds"`
		Paused        bool   `json:"paused"`
		Engine        string `json:"engine"`
		Mode          string `json:"mode"`
		MinRows       *int64 `json:"min_rows"`
		Reads         string `json:"reads"`
	} `json:"sql"`
}

type sinkModel struct {
	ID              types.String `tfsdk:"id"`
	Catalog         types.String `tfsdk:"catalog"`
	Name            types.String `tfsdk:"name"`
	Table           types.String `tfsdk:"table"`
	SQL             types.String `tfsdk:"sql"`
	Every           types.String `tfsdk:"every"`
	Offset          types.String `tfsdk:"offset"`
	Mode            types.String `tfsdk:"mode"`
	MinRows         types.Int64  `tfsdk:"min_rows"`
	Key             types.List   `tfsdk:"key"`
	Engine          types.String `tfsdk:"engine"`
	Paused          types.Bool   `tfsdk:"paused"`
	CreateTable     types.Bool   `tfsdk:"create_table"`
	FormatVersion   types.Int64  `tfsdk:"format_version"`
	ReplaceExisting types.Bool   `tfsdk:"replace_existing"`
	Reads           types.String `tfsdk:"reads"`
}

type sinkResource struct{ c *Client }

func newSinkResource() resource.Resource { return &sinkResource{} }

func (r *sinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_sql_sink"
}

func (r *sinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A scheduled SQL sink: one DuckDB SELECT (over URLs, or over the catalog's own tables) run on a " +
			"schedule, its rows committed to one table. Its query is previewed before it is made or changed. Destroying " +
			"it deletes the sink only: its table and rows stay.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"catalog": schema.StringAttribute{Required: true, Description: "The catalog's name or id.", PlanModifiers: replace},
			"name":    schema.StringAttribute{Required: true, PlanModifiers: replace},
			"table":   schema.StringAttribute{Required: true, Description: "namespace.table.", PlanModifiers: replace},
			"sql":     schema.StringAttribute{Required: true, Description: "The SELECT (file(\"sql/x.sql\") keeps it in a file)."},
			"every":   schema.StringAttribute{Required: true, Description: "15m, 1h, 1d…"},
			"offset":  schema.StringAttribute{Optional: true, Description: "Where in the interval it runs (4m)."},
			"mode": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "append (default) or replace (each run replaces the table; v2 tables).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()}},
			"min_rows": schema.Int64Attribute{Optional: true, Description: "A replace run with fewer rows stores nothing."},
			"key":      schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Rows sharing it replace each other."},
			"engine": schema.StringAttribute{Optional: true, Computed: true, Description: "2.0 (default) or 1.5.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"paused": schema.BoolAttribute{Optional: true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			// creation-time only: used when the sink is made, never read back, never a replacement (an imported sink
			// whose configuration sets them is not recreated; high review of #3032)
			"create_table": schema.BoolAttribute{Optional: true, Description: "When it is made: make the table from the " +
				"query's columns if it does not exist (prefer migrations)."},
			"format_version": schema.Int64Attribute{Optional: true, Description: "When it is made: the format version of a table it makes."},
			"replace_existing": schema.BoolAttribute{Optional: true,
				Description: "When it is made: a replace sink on a table that holds rows replaces them at its first run."},
			"reads": schema.StringAttribute{Computed: true, Description: "urls, or catalog (a derived sink).",
				PlanModifiers: []planmodifier.String{sameWhileSQL{}}},
		},
	}
}

func (r *sinkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

func (r *sinkResource) get(ctx context.Context, id string) (*sinkAPI, error) {
	var a sinkAPI
	err := r.c.Do(ctx, "GET", "/v1/sinks/"+id, nil, nil, &a)
	return &a, err
}

// from keeps what the configuration says where the live value means the same (a duration spelled 15m is 900 s)
func (m *sinkModel) from(ctx context.Context, a *sinkAPI) {
	m.ID, m.Name, m.Table = types.StringValue(a.SinkID), types.StringValue(a.Name), types.StringValue(a.Table)
	if a.SQL == nil {
		return
	}
	q := a.SQL
	if strings.TrimSpace(m.SQL.ValueString()) != strings.TrimSpace(q.Query) {
		m.SQL = types.StringValue(q.Query)
	}
	if s, err := seconds(m.Every.ValueString()); err != nil || s != q.EverySeconds {
		m.Every = types.StringValue(itoa(q.EverySeconds) + "s")
	}
	if s, err := seconds(m.Offset.ValueString()); m.Offset.IsNull() && q.OffsetSeconds != 0 || !m.Offset.IsNull() && (err != nil || s != q.OffsetSeconds) {
		m.Offset = types.StringValue(itoa(q.OffsetSeconds) + "s")
	}
	mode := q.Mode
	if mode == "" {
		mode = "append"
	}
	m.Mode, m.Engine, m.Paused, m.Reads = types.StringValue(mode), types.StringValue(q.Engine), types.BoolValue(q.Paused),
		types.StringValue(q.Reads)
	if q.MinRows != nil && (!m.MinRows.IsNull() || *q.MinRows != 1) {
		m.MinRows = types.Int64Value(*q.MinRows) // the default guard (1) of a configuration that sets none stays null
	}
	if !m.Key.IsNull() || len(a.Key) > 0 {
		v, _ := types.ListValueFrom(ctx, types.StringType, a.Key)
		if len(a.Key) == 0 {
			v = types.ListNull(types.StringType)
		}
		m.Key = v
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func (r *sinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m sinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": m.Name.ValueString(), "warehouse": m.Catalog.ValueString(), "table": m.Table.ValueString(),
		"sql": m.SQL.ValueString(), "every": m.Every.ValueString()}
	if !m.Offset.IsNull() {
		body["offset"] = m.Offset.ValueString()
	}
	if !m.Mode.IsNull() && !m.Mode.IsUnknown() {
		body["mode"] = m.Mode.ValueString()
	}
	if !m.MinRows.IsNull() {
		body["min_rows"] = m.MinRows.ValueInt64()
	}
	if !m.Engine.IsNull() && !m.Engine.IsUnknown() {
		body["engine"] = m.Engine.ValueString()
	}
	if !m.Paused.IsNull() && !m.Paused.IsUnknown() {
		body["paused"] = m.Paused.ValueBool()
	}
	if !m.CreateTable.IsNull() {
		body["create_table"] = m.CreateTable.ValueBool()
	}
	if !m.FormatVersion.IsNull() {
		body["format_version"] = m.FormatVersion.ValueInt64()
	}
	if !m.ReplaceExisting.IsNull() {
		body["replace_existing"] = m.ReplaceExisting.ValueBool()
	}
	if !m.Key.IsNull() {
		var k []string
		resp.Diagnostics.Append(m.Key.ElementsAs(ctx, &k, false)...)
		body["key"] = k
	}
	var made sinkAPI
	if err := r.c.Do(ctx, "POST", "/v1/sinks", nil, body, &made); err != nil {
		apiErr(&resp.Diagnostics, "create sink "+m.Name.ValueString(), err)
		return
	}
	m.ID = types.StringValue(made.SinkID)
	if a, err := r.get(ctx, made.SinkID); err == nil {
		m.from(ctx, a)
	} else {
		// made: it goes into state whatever the read says, or the next apply would make it again (409)
		resp.Diagnostics.AddWarning("LakehouseBox: read the new sink", err.Error())
		for _, u := range []*types.String{&m.Reads, &m.Engine} {
			if u.IsUnknown() {
				*u = types.StringNull()
			}
		}
		if m.Mode.IsUnknown() {
			m.Mode = types.StringValue("append")
		}
		if m.Paused.IsUnknown() {
			m.Paused = types.BoolValue(false)
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m sinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	a, err := r.get(ctx, m.ID.ValueString())
	if NotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErr(&resp.Diagnostics, "read sink", err)
		return
	}
	m.from(ctx, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, prior sinkModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	patch := map[string]any{}
	if m.SQL.ValueString() != prior.SQL.ValueString() {
		patch["sql"] = m.SQL.ValueString()
	}
	if m.Every.ValueString() != prior.Every.ValueString() {
		patch["every"] = m.Every.ValueString()
	}
	if !m.Offset.Equal(prior.Offset) {
		off := m.Offset.ValueString()
		if m.Offset.IsNull() {
			off = "0"
		}
		patch["offset"] = off
	}
	if !m.MinRows.Equal(prior.MinRows) {
		if m.MinRows.IsNull() {
			patch["min_rows"] = 1 // the guard's default: removing it from the configuration resets it
		} else {
			patch["min_rows"] = m.MinRows.ValueInt64()
		}
	}
	if !m.Engine.IsUnknown() && !m.Engine.IsNull() && !m.Engine.Equal(prior.Engine) {
		patch["engine"] = m.Engine.ValueString()
	}
	if !m.Paused.IsUnknown() && !m.Paused.IsNull() && !m.Paused.Equal(prior.Paused) {
		patch["paused"] = m.Paused.ValueBool()
	}
	if !m.Key.Equal(prior.Key) {
		var k []string
		if !m.Key.IsNull() {
			resp.Diagnostics.Append(m.Key.ElementsAs(ctx, &k, false)...)
		}
		patch["key"] = k
	}
	if len(patch) > 0 {
		if err := r.c.Do(ctx, "PATCH", "/v1/sinks/"+prior.ID.ValueString(), nil, patch, nil); err != nil {
			apiErr(&resp.Diagnostics, "update sink "+m.Name.ValueString(), err)
			return
		}
	}
	a, err := r.get(ctx, prior.ID.ValueString())
	if err != nil {
		apiErr(&resp.Diagnostics, "read sink", err)
		return
	}
	m.ID = prior.ID
	m.from(ctx, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *sinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m sinkModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if err := r.c.Do(ctx, "DELETE", "/v1/sinks/"+m.ID.ValueString(), nil, nil, nil); err != nil && !NotFound(err) {
		apiErr(&resp.Diagnostics, "delete sink", err)
	}
}

func (r *sinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// import by id: `tofu import lakehousebox_sql_sink.x <sink id>`
	a, err := r.get(ctx, req.ID)
	if err != nil {
		apiErr(&resp.Diagnostics, "import sink "+req.ID, err)
		return
	}
	m := sinkModel{Catalog: types.StringValue(a.WarehouseName), Key: types.ListNull(types.StringType),
		Offset: types.StringNull(), MinRows: types.Int64Null()}
	if a.SQL != nil {
		m.SQL, m.Every = types.StringValue(a.SQL.Query), types.StringValue(itoa(a.SQL.EverySeconds)+"s")
	}
	m.from(ctx, a)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// sameWhileSQL: `reads` is the API's verdict on the query, so it stays as it was while `sql` does not change, and is
// unknown (decided by the API) when it does (high review of #3032).
type sameWhileSQL struct{}

func (sameWhileSQL) Description(context.Context) string { return "unchanged while sql is unchanged" }
func (sameWhileSQL) MarkdownDescription(context.Context) string {
	return "unchanged while sql is unchanged"
}
func (sameWhileSQL) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || !req.PlanValue.IsUnknown() {
		return
	}
	var planSQL, stateSQL types.String
	req.Plan.GetAttribute(ctx, path.Root("sql"), &planSQL)
	req.State.GetAttribute(ctx, path.Root("sql"), &stateSQL)
	if planSQL.Equal(stateSQL) {
		resp.PlanValue = req.StateValue
	}
}
