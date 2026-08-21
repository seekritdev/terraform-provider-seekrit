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
	_ datasource.DataSource                   = &groupDataSource{}
	_ datasource.DataSourceWithConfigure      = &groupDataSource{}
	_ datasource.DataSourceWithValidateConfig = &groupDataSource{}
)

// NewGroupDataSource is the seekrit_group data source factory.
func NewGroupDataSource() datasource.DataSource { return &groupDataSource{} }

type groupDataSource struct{ data *providerData }

type groupDataSourceModel struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Slug      types.String `tfsdk:"slug"`
	CreatedAt types.String `tfsdk:"created_at"`
	UpdatedAt types.String `tfsdk:"updated_at"`
}

func (d *groupDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_group"
}

func (d *groupDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An existing group (a reusable, org-scoped secret bag), looked up by " +
			"`slug` or `id`. Use it to compose a dashboard-created group into a " +
			"Terraform-managed environment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The group id (`grp_…`). Set this or `slug`, not both.",
			},
			"slug": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The group slug, unique within the org. Set this or `id`, not both.",
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

func (d *groupDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	requireExactlyOneLookupKey(&resp.Diagnostics, "group", cfg.ID, cfg.Slug)
}

func (d *groupDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = configureDataSource(req, resp)
}

func (d *groupDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg groupDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var group *client.Group
	var err error
	if !cfg.ID.IsNull() {
		group, err = d.data.client.GetGroup(ctx, d.data.orgID, cfg.ID.ValueString())
	} else {
		group, err = findGroupBySlug(ctx, d.data, cfg.Slug.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading group", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, groupDataSourceModel{
		ID:        types.StringValue(group.ID),
		Name:      types.StringValue(group.Name),
		Slug:      types.StringValue(group.Slug),
		CreatedAt: types.StringValue(group.CreatedAt),
		UpdatedAt: types.StringValue(group.UpdatedAt),
	})...)
}

func findGroupBySlug(ctx context.Context, data *providerData, slug string) (*client.Group, error) {
	groups, err := data.client.ListGroups(ctx, data.orgID)
	if err != nil {
		return nil, err
	}
	for i := range groups {
		if groups[i].Slug == slug {
			return &groups[i], nil
		}
	}
	return nil, fmt.Errorf("no group with slug %q in this organization", slug)
}
