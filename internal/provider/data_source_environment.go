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
	_ datasource.DataSource                   = &environmentDataSource{}
	_ datasource.DataSourceWithConfigure      = &environmentDataSource{}
	_ datasource.DataSourceWithValidateConfig = &environmentDataSource{}
)

// NewEnvironmentDataSource is the seekrit_environment data source factory.
func NewEnvironmentDataSource() datasource.DataSource { return &environmentDataSource{} }

type environmentDataSource struct{ data *providerData }

type environmentDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	GroupID       types.String `tfsdk:"group_id"`
	Slug          types.String `tfsdk:"slug"`
	Name          types.String `tfsdk:"name"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func (d *environmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An existing environment, looked up either by `id` or by its owner " +
			"(`application_id` or `group_id`) plus `slug`. The pair is how you address an " +
			"environment you did not create in Terraform — for example to grant a " +
			"Terraform-managed token access to the `production` environment of an app the " +
			"dashboard owns.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The environment id (`env_…`). Set this, or an owner plus `slug` — " +
					"not both.",
			},
			"application_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The owning application id, when looking up by owner + `slug`.",
			},
			"group_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The owning group id, when looking up by owner + `slug`.",
			},
			"slug": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The environment slug, unique within its owner.",
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

func (d *environmentDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg environmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	byID := !cfg.ID.IsNull()
	byOwner := !cfg.ApplicationID.IsNull() || !cfg.GroupID.IsNull()
	switch {
	case byID && byOwner:
		resp.Diagnostics.AddError(
			"Ambiguous environment lookup",
			"Set `id`, or an owner (`application_id`/`group_id`) plus `slug` — not both.",
		)
	case !byID && !byOwner:
		resp.Diagnostics.AddError(
			"Missing environment lookup",
			"Set `id`, or an owner (`application_id`/`group_id`) plus `slug`.",
		)
	case byOwner && !cfg.ApplicationID.IsNull() && !cfg.GroupID.IsNull():
		resp.Diagnostics.AddError(
			"Invalid environment owner",
			"An environment is owned by exactly one of an application or a group; set only one.",
		)
	case byOwner && cfg.Slug.IsNull():
		resp.Diagnostics.AddError(
			"Missing environment slug",
			"Looking up by owner also needs `slug` — an owner has several environments.",
		)
	}
}

func (d *environmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = configureDataSource(req, resp)
}

func (d *environmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg environmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var env *client.Environment
	var err error
	switch {
	case !cfg.ID.IsNull():
		env, err = d.data.client.GetEnvironment(ctx, d.data.orgID, cfg.ID.ValueString())
	case !cfg.ApplicationID.IsNull():
		env, err = findEnvBySlug(ctx, d.data, "application", cfg.ApplicationID.ValueString(), cfg.Slug.ValueString())
	default:
		env, err = findEnvBySlug(ctx, d.data, "group", cfg.GroupID.ValueString(), cfg.Slug.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Error reading environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, environmentDataSourceModel{
		ID:            types.StringValue(env.ID),
		ApplicationID: stringOrNull(env.ApplicationID),
		GroupID:       stringOrNull(env.GroupID),
		Slug:          types.StringValue(env.Slug),
		Name:          types.StringValue(env.Name),
		CreatedAt:     types.StringValue(env.CreatedAt),
		UpdatedAt:     types.StringValue(env.UpdatedAt),
	})...)
}

// findEnvBySlug lists an owner's environments and picks the matching slug. The
// API has no lookup-by-slug route, and an owner's env list is short.
func findEnvBySlug(ctx context.Context, data *providerData, ownerKind, ownerID, slug string) (*client.Environment, error) {
	var envs []client.Environment
	var err error
	if ownerKind == "application" {
		envs, err = data.client.ListAppEnvs(ctx, data.orgID, ownerID)
	} else {
		envs, err = data.client.ListGroupEnvs(ctx, data.orgID, ownerID)
	}
	if err != nil {
		return nil, err
	}
	for i := range envs {
		if envs[i].Slug == slug {
			return &envs[i], nil
		}
	}
	return nil, fmt.Errorf("no environment with slug %q in %s %q", slug, ownerKind, ownerID)
}
