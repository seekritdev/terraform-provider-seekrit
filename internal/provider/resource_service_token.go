package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
	"github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
)

var (
	_ resource.Resource                = &serviceTokenResource{}
	_ resource.ResourceWithConfigure   = &serviceTokenResource{}
	_ resource.ResourceWithImportState = &serviceTokenResource{}
)

// NewServiceTokenResource is the seekrit_service_token resource factory.
func NewServiceTokenResource() resource.Resource { return &serviceTokenResource{} }

type serviceTokenResource struct{ data *providerData }

type serviceTokenModel struct {
	ID            types.String `tfsdk:"id"`
	Name          types.String `tfsdk:"name"`
	Role          types.String `tfsdk:"role"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	ExpiresAt     types.String `tfsdk:"expires_at"`
	Token         types.String `tfsdk:"token"`
	TokenHash     types.String `tfsdk:"token_hash"`
	PublicKeyJwk  types.String `tfsdk:"public_key_jwk"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (r *serviceTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_token"
}

func (r *serviceTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A seekrit service token. The provider mints the keypair and token string " +
			"locally; the server only ever receives the hash and public key. The secret `token` is " +
			"returned once and stored (sensitive) in Terraform state — like any Terraform-created " +
			"credential (e.g. `aws_iam_access_key.secret`). Use an encrypted state backend.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The public token id (`skt_…`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable token name (editable in place).",
			},
			"role": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Org capability: `member` (default) or `admin`. Immutable — " +
					"changing it mints a new token.",
				Validators:    []validator.String{stringvalidator.OneOf("member", "admin")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
			},
			"environment_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The application environment (`env_…`) this token is bound to. " +
					"Immutable — changing it mints a new token.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"expires_at": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Optional expiry (ISO 8601). Immutable — changing it mints a new token.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The full secret token string (`skt_…_…`), returned only at creation.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"token_hash": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "SHA-256 (base64url) of the token — what the server stores.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"public_key_jwk": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The token's public key (JWK), used to wrap DEK grants for it.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (ISO 8601).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *serviceTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = configureResource(req, resp)
}

func (r *serviceTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := crypto.CreateServiceToken()
	if err != nil {
		resp.Diagnostics.AddError("Error minting service token", err.Error())
		return
	}
	role := "member"
	if !plan.Role.IsNull() && !plan.Role.IsUnknown() {
		role = plan.Role.ValueString()
	}
	tok, err := r.data.client.CreateToken(ctx, r.data.orgID, client.CreateTokenInput{
		Name:          plan.Name.ValueString(),
		TokenID:       created.TokenID,
		TokenHash:     created.TokenHash,
		PublicKeyJwk:  created.PublicKeyJWK,
		Role:          role,
		EnvironmentID: optionalString(plan.EnvironmentID),
		ExpiresAt:     optionalString(plan.ExpiresAt),
	})
	if err != nil {
		resp.Diagnostics.AddError("Error creating service token", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, serviceTokenModel{
		ID:            types.StringValue(tok.ID),
		Name:          types.StringValue(tok.Name),
		Role:          types.StringValue(tok.Role),
		EnvironmentID: stringOrNull(tok.EnvironmentID),
		ExpiresAt:     stringOrNull(tok.ExpiresAt),
		Token:         types.StringValue(created.Token),
		TokenHash:     types.StringValue(created.TokenHash),
		PublicKeyJwk:  types.StringValue(tok.PublicKeyJwk),
		CreatedAt:     types.StringValue(tok.CreatedAt),
	})...)
}

func (r *serviceTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state serviceTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tok, err := r.data.client.GetToken(ctx, r.data.orgID, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading service token", err.Error())
		return
	}
	// A missing or revoked token is gone as far as Terraform is concerned; the
	// secret string is unrecoverable, so recreation is the only path back.
	if tok == nil || tok.RevokedAt != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	// Refresh server-owned fields; the secret token + hash are never returned by
	// the API, so preserve whatever is already in state.
	state.Name = types.StringValue(tok.Name)
	state.Role = types.StringValue(tok.Role)
	state.EnvironmentID = stringOrNull(tok.EnvironmentID)
	state.ExpiresAt = stringOrNull(tok.ExpiresAt)
	state.PublicKeyJwk = types.StringValue(tok.PublicKeyJwk)
	state.CreatedAt = types.StringValue(tok.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *serviceTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan serviceTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only name is mutable in place; everything else is RequiresReplace.
	tok, err := r.data.client.UpdateToken(ctx, r.data.orgID, plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating service token", err.Error())
		return
	}
	plan.Name = types.StringValue(tok.Name)
	plan.Role = types.StringValue(tok.Role)
	plan.EnvironmentID = stringOrNull(tok.EnvironmentID)
	plan.ExpiresAt = stringOrNull(tok.ExpiresAt)
	plan.PublicKeyJwk = types.StringValue(tok.PublicKeyJwk)
	plan.CreatedAt = types.StringValue(tok.CreatedAt)
	// plan.ID, plan.Token, plan.TokenHash carry over from state (UseStateForUnknown).
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state serviceTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.RevokeToken(ctx, r.data.orgID, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking service token", err.Error())
	}
}

func (r *serviceTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The secret token string cannot be recovered on import — token/token_hash
	// will be null afterward.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
