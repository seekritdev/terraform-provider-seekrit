package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = &organizationDataSource{}
	_ datasource.DataSourceWithConfigure = &organizationDataSource{}
)

// NewOrganizationDataSource is the seekrit_organization data source factory.
func NewOrganizationDataSource() datasource.DataSource { return &organizationDataSource{} }

type organizationDataSource struct{ data *providerData }

type organizationDataSourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Slug types.String `tfsdk:"slug"`
	Role types.String `tfsdk:"role"`
}

func (d *organizationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization"
}

func (d *organizationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The organization the configured token belongs to. Takes no arguments — " +
			"a service token is scoped to exactly one org, so there is nothing to look up. " +
			"Useful for asserting a configuration is pointed at the org you think it is, and " +
			"for reading back `role` to confirm the token really is an admin.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The organization id (`org_…`).",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name.",
			},
			"slug": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "URL-safe identifier, globally unique.",
			},
			"role": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "The configured token's role in this org (`admin` or `member`). " +
					"Most resources in this provider need `admin`.",
			},
		},
	}
}

func (d *organizationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = configureDataSource(req, resp)
}

func (d *organizationDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	org, err := d.data.client.GetOrganization(ctx, d.data.orgID)
	if err != nil {
		resp.Diagnostics.AddError("Error reading organization", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, organizationDataSourceModel{
		ID:   types.StringValue(org.ID),
		Name: types.StringValue(org.Name),
		Slug: types.StringValue(org.Slug),
		Role: types.StringValue(org.Role),
	})...)
}
