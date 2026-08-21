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
	_ datasource.DataSource                   = &serviceTokenDataSource{}
	_ datasource.DataSourceWithConfigure      = &serviceTokenDataSource{}
	_ datasource.DataSourceWithValidateConfig = &serviceTokenDataSource{}
)

// NewServiceTokenDataSource is the seekrit_service_token data source factory.
func NewServiceTokenDataSource() datasource.DataSource { return &serviceTokenDataSource{} }

type serviceTokenDataSource struct{ data *providerData }

type serviceTokenDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Role          types.String `tfsdk:"role"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	PublicKeyJwk  types.String `tfsdk:"public_key_jwk"`
	ExpiresAt     types.String `tfsdk:"expires_at"`
	LastUsedAt    types.String `tfsdk:"last_used_at"`
	RevokedAt     types.String `tfsdk:"revoked_at"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (d *serviceTokenDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_token"
}

func (d *serviceTokenDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "An existing service token's public metadata, looked up by `id` or `name`. " +
			"The secret token string is **not** available — it exists only in the response to the " +
			"call that created it. What this is for is `public_key_jwk`: the key a DEK grant is " +
			"wrapped to, so a token minted in the dashboard or by the CLI can be granted access " +
			"to a Terraform-managed environment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The token id (`skt_…`). Set this or `name`, not both.",
			},
			"name": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The token's display name. Set this or `id`, not both. Names are " +
					"not unique — a lookup that matches more than one active token is an error.",
			},
			"role": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Org capability: `admin` or `member`.",
			},
			"environment_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The application environment this token is bound to, if any.",
			},
			"public_key_jwk": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The token's public key (JWK) — what a DEK grant is wrapped to.",
			},
			"expires_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Expiry timestamp (ISO 8601), if set.",
			},
			"last_used_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the token last authenticated a request, if ever.",
			},
			"revoked_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the token was revoked, if it has been.",
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (ISO 8601).",
			},
		},
	}
}

func (d *serviceTokenDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var cfg serviceTokenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	requireExactlyOneLookupKey(&resp.Diagnostics, "service token", cfg.ID, cfg.Name)
}

func (d *serviceTokenDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.data = configureDataSource(req, resp)
}

func (d *serviceTokenDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serviceTokenDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tokens, err := d.data.client.ListTokens(ctx, d.data.orgID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing service tokens", err.Error())
		return
	}

	var matches []client.ServiceToken
	for _, t := range tokens {
		if !cfg.ID.IsNull() {
			if t.ID == cfg.ID.ValueString() {
				matches = append(matches, t)
			}
			continue
		}
		// Revoked tokens keep their name but authenticate nothing, so a
		// name lookup ignores them rather than reporting a false ambiguity.
		if t.Name == cfg.Name.ValueString() && t.RevokedAt == nil {
			matches = append(matches, t)
		}
	}
	switch len(matches) {
	case 0:
		resp.Diagnostics.AddError("Service token not found",
			fmt.Sprintf("No active service token in this org matches %s.", tokenLookupLabel(cfg)))
		return
	case 1:
	default:
		resp.Diagnostics.AddError("Ambiguous service token lookup",
			fmt.Sprintf("%d active tokens match %s. Look the token up by `id` instead.",
				len(matches), tokenLookupLabel(cfg)))
		return
	}

	tok := matches[0]
	resp.Diagnostics.Append(resp.State.Set(ctx, serviceTokenDataSourceModel{
		ID:            types.StringValue(tok.ID),
		Name:          types.StringValue(tok.Name),
		Role:          types.StringValue(tok.Role),
		EnvironmentID: stringOrNull(tok.EnvironmentID),
		PublicKeyJwk:  types.StringValue(tok.PublicKeyJwk),
		ExpiresAt:     stringOrNull(tok.ExpiresAt),
		LastUsedAt:    stringOrNull(tok.LastUsedAt),
		RevokedAt:     stringOrNull(tok.RevokedAt),
		CreatedAt:     types.StringValue(tok.CreatedAt),
	})...)
}

func tokenLookupLabel(cfg serviceTokenDataSourceModel) string {
	if !cfg.ID.IsNull() {
		return fmt.Sprintf("id %q", cfg.ID.ValueString())
	}
	return fmt.Sprintf("name %q", cfg.Name.ValueString())
}
