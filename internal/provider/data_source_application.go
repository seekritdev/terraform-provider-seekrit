package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
)

var (
	_ datasource.DataSource                   = &applicationDataSource{}
	_ datasource.DataSourceWithConfigure      = &applicationDataSource{}
	_ datasource.DataSourceWithValidateConfig = &applicationDataSource{}
)

// NewApplicationDataSource is the seekrit_application data source factory.
func NewApplicationDataSource() datasource.DataSource { return &applicationDataSource{} }

type applicationDataSource struct{ data *providerData }

type applicationDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Slug      types.String `tfsdk:"slug"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (d *applicationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_application"
}

func (d *applicationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An existing application, looked up by `slug` or `id`. Use this to attach " +
			"Terraform-managed environments to an application that was created in the dashboard, " +
			"without importing the application itself.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The application id (`app_…`). Set this or `slug`, not both.",
			},
			"slug": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The application slug, unique within the org. Set this or `id`, not both.",
			},
			"name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Display name.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (ISO 8601).",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last-update timestamp (ISO 8601).",
			},
		},
	}
}

func (d *applicationDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg applicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	requireExactlyOneLookupKey(&resp.Diagnostics, "application", cfg.ID, cfg.Slug)
}

func (d *applicationDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = configureDataSource(req, resp)
}

func (d *applicationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg applicationDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var app *client.Application
	var err error
	if !cfg.ID.IsNull() {
		app, err = d.data.client.GetApplication(ctx, d.data.orgID, cfg.ID.ValueString())
	} else {
		app, err = findApplicationBySlug(ctx, d.data, cfg.Slug.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading application", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, applicationDataSourceModel{
		ID:        types.StringValue(app.ID),
		Name:      types.StringValue(app.Name),
		Slug:      types.StringValue(app.Slug),
		CreatedAt: types.StringValue(app.CreatedAt),
		UpdatedAt: types.StringValue(app.UpdatedAt),
	})...)
}

// findApplicationBySlug resolves a slug through the org's app list — the API has
// no lookup-by-slug route, and the list is small and org-scoped.
func findApplicationBySlug(ctx context.Context, data *providerData, slug string) (*client.Application, error) {
	apps, err := data.client.ListApplications(ctx, data.orgID)
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Slug == slug {
			return &apps[i], nil
		}
	}
	return nil, fmt.Errorf("no application with slug %q in this organization", slug)
}
