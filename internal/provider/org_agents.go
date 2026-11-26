package provider

// lakehousebox_org_agent_settings: what an organisation's admins decide about its hosted agents and analysts
// (research/156; GET/PATCH /v1/orgs/{org}/agent-settings): the models they may use, the default model, and the
// instructions every agent and analyst is given. One per organisation: destroying it puts the settings back to the
// deployment's (every model on, its default, no instructions).

import (
	"context"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type orgAgentsModel struct {
	ID           types.String `tfsdk:"id"`
	Org          types.String `tfsdk:"org"`
	Models       types.Set    `tfsdk:"models"`
	DefaultModel types.String `tfsdk:"default_model"`
	Instructions types.String `tfsdk:"instructions"`
}

type orgAgentsAPI struct {
	OrgID  string `json:"org_id"`
	Models []struct {
		Model   string `json:"model"`
		Allowed bool   `json:"allowed"`
	} `json:"models"`
	DefaultModel string `json:"default_model"`
	Instructions struct {
		Text *string `json:"text"`
	} `json:"instructions"`
}

type orgAgentsResource struct{ c *Client }

func newOrgAgentSettingsResource() resource.Resource { return &orgAgentsResource{} }

func (r *orgAgentsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_agent_settings"
}

func (r *orgAgentsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "What an organisation's admins decide about its hosted agents and analysts: the models they may use, " +
			"the default model, and the instructions every agent and analyst is given. Needs a key with the admin role. " +
			"Destroying it puts back the deployment's settings: every model on, its default, no instructions.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			// not RequiresReplace: an import by id under a configuration that names the handle would destroy (reset) and
			// recreate the settings; a real move to another organisation is handled in Update (high review of #4021)
			"org": schema.StringAttribute{Required: true, Description: "The organisation's handle or id."},
			"models": schema.SetAttribute{Optional: true, Computed: true, ElementType: types.StringType,
				Description: "The models agents may use (`lhbox agent models` lists them); the others are switched off. " +
					"Left out: whatever is set now.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()}},
			"default_model": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "The model a new agent gets and every analyst runs on; one of `models`. Left out: whatever is set now.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"instructions": schema.StringAttribute{Optional: true,
				Description: "Given to every agent and analyst after LakehouseBox's own rules and before its task, 4,000 " +
					"characters at most; file(\"agents/instructions.md\"). Left out: none."},
		},
	}
}

func (r *orgAgentsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = configure(req, resp)
}

// fill sets the model from what the API answers. `instructions` is the configuration's when the API's text is the
// same once trimmed (the API keeps it trimmed), so a file ending in a newline plans no change.
func (m *orgAgentsModel) fill(ctx context.Context, a *orgAgentsAPI) {
	on := []string{}
	for _, x := range a.Models {
		if x.Allowed {
			on = append(on, x.Model)
		}
	}
	sort.Strings(on)
	m.Models, _ = types.SetValueFrom(ctx, types.StringType, on)
	m.DefaultModel = types.StringValue(a.DefaultModel)
	m.ID = types.StringValue(a.OrgID)
	switch {
	case a.Instructions.Text == nil && !m.Instructions.IsNull() && !m.Instructions.IsUnknown() && trimmed(m.Instructions.ValueString()) == "":
		// "" or a file of blank lines: stored as none; the configuration's value stays, so the apply is consistent
	case a.Instructions.Text == nil:
		m.Instructions = types.StringNull()
	case !m.Instructions.IsNull() && !m.Instructions.IsUnknown() && trimmed(m.Instructions.ValueString()) == *a.Instructions.Text:
		// keep the configuration's spelling
	default:
		m.Instructions = types.StringValue(*a.Instructions.Text)
	}
}

func trimmed(s string) string { return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n")) }

// patch sends what the CONFIGURATION sets (`cfg`), never a value the plan carries over from state: a default left out
// stays the deployment's (null), and dropping it from `models` lets the API pick, rather than a stale name being sent
// back as if chosen (high review of #4021). The instructions are always sent: left out means none.
func (r *orgAgentsResource) patch(ctx context.Context, m *orgAgentsModel, cfg *orgAgentsModel) (*orgAgentsAPI, error) {
	body := map[string]any{"instructions": nil}
	if !cfg.Instructions.IsNull() && !cfg.Instructions.IsUnknown() {
		body["instructions"] = cfg.Instructions.ValueString()
	}
	if !cfg.Models.IsNull() && !cfg.Models.IsUnknown() {
		var ms []string
		cfg.Models.ElementsAs(ctx, &ms, false)
		body["models"] = ms
	}
	if !cfg.DefaultModel.IsNull() && !cfg.DefaultModel.IsUnknown() {
		body["default_model"] = cfg.DefaultModel.ValueString()
	}
	var out orgAgentsAPI
	err := r.c.Do(ctx, "PATCH", orgPath(m.Org.ValueString(), "/agent-settings"), nil, body, &out)
	return &out, err
}

func (r *orgAgentsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var m, cfg orgAgentsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	out, err := r.patch(ctx, &m, &cfg)
	if err != nil {
		apiErr(&resp.Diagnostics, "set the organisation's agent settings", err)
		return
	}
	m.fill(ctx, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *orgAgentsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var m orgAgentsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	var out orgAgentsAPI
	if err := r.c.Do(ctx, "GET", orgPath(m.Org.ValueString(), "/agent-settings"), nil, nil, &out); err != nil {
		if orgGone(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "read the organisation's agent settings", err)
		return
	}
	m.fill(ctx, &out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func (r *orgAgentsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var m, cfg, prev orgAgentsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &m)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prev)...)
	out, err := r.patch(ctx, &m, &cfg)
	if err != nil {
		apiErr(&resp.Diagnostics, "change the organisation's agent settings", err)
		return
	}
	if old := prev.ID.ValueString(); old != "" && out.OrgID != "" && old != out.OrgID {
		// moved to another organisation: the one it managed goes back to the deployment's settings, as a destroy would
		if err := r.reset(ctx, old); err != nil {
			apiErr(&resp.Diagnostics, "reset the organisation it managed before", err)
		}
	}
	m.fill(ctx, out)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// `tofu import lakehousebox_org_agent_settings.x <org>`; Read then fills the rest.
func (r *orgAgentsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.Set(ctx, &orgAgentsModel{ID: types.StringValue(req.ID), Org: types.StringValue(req.ID),
		Models: types.SetNull(types.StringType), DefaultModel: types.StringNull(), Instructions: types.StringNull()})...)
}

// orgGone: the organisation itself is gone (deleted, or the key is no longer in it): 404 org_not_found, read here only
// (a wrong endpoint answers a bare not_found, which never empties state).
func orgGone(err error) bool {
	e, ok := err.(*APIError)
	return NotFound(err) || (ok && e.Status == 404 && e.Code == "org_not_found")
}

// reset puts back the deployment's settings of an organisation: every model on, its default, no instructions.
func (r *orgAgentsResource) reset(ctx context.Context, org string) error {
	var cur orgAgentsAPI
	if err := r.c.Do(ctx, "GET", orgPath(org, "/agent-settings"), nil, nil, &cur); err != nil {
		if orgGone(err) {
			return nil
		}
		return err
	}
	all := []string{}
	for _, x := range cur.Models {
		all = append(all, x.Model)
	}
	body := map[string]any{"models": all, "default_model": nil, "instructions": nil}
	if err := r.c.Do(ctx, "PATCH", orgPath(org, "/agent-settings"), nil, body, nil); err != nil && !orgGone(err) {
		return err
	}
	return nil
}

// Delete puts back the deployment's settings: every model on, its default, no instructions.
func (r *orgAgentsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var m orgAgentsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &m)...)
	if err := r.reset(ctx, m.Org.ValueString()); err != nil {
		apiErr(&resp.Diagnostics, "reset the organisation's agent settings", err)
	}
}
