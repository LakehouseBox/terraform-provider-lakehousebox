package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type lhProvider struct{ version string }

// New is the provider factory providerserver serves.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &lhProvider{version: version} }
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	APIKey   types.String `tfsdk:"api_key"`
}

func (p *lhProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "lakehousebox"
	resp.Version = p.version
}

func (p *lhProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "LakehouseBox: managed Apache Iceberg catalogs in Europe. Manages catalogs and what feeds them " +
			"(scheduled SQL sinks, connections, POST prefixes, AGENTS.md, table descriptions). Tables come from " +
			"migrations (`lhbox migrate`), never from this provider.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{Optional: true,
				Description: "The API, default https://api.lakehousebox.com (or LHBOX_URL)."},
			"api_key": schema.StringAttribute{Optional: true, Sensitive: true,
				Description: "An API key or agent key (or LHBOX_API_KEY). Never write it in a file: use the environment."},
		},
	}
}

func (p *lhProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := m.Endpoint.ValueString()
	if endpoint == "" {
		endpoint = os.Getenv("LHBOX_URL")
	}
	if endpoint == "" {
		endpoint = "https://api.lakehousebox.com"
	}
	key := m.APIKey.ValueString()
	if key == "" {
		key = os.Getenv("LHBOX_API_KEY")
	}
	if key == "" {
		resp.Diagnostics.AddError("No API key", "Set LHBOX_API_KEY (an API key or agent key; `lhbox login` makes one) "+
			"or the provider's api_key.")
		return
	}
	c := newClient(endpoint, key, p.version)
	resp.ResourceData, resp.DataSourceData = c, c
}

func (p *lhProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{newCatalogResource, newPostPrefixResource, newConnectionResource,
		newSinkResource, newAgentsMDResource, newTableDescriptionResource}
}

func (p *lhProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newCatalogDataSource}
}
