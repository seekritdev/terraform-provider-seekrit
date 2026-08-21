package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &environmentKeyGrantResource{}
	_ resource.ResourceWithConfigure   = &environmentKeyGrantResource{}
	_ resource.ResourceWithImportState = &environmentKeyGrantResource{}
)

// NewEnvironmentKeyGrantResource is the seekrit_environment_key_grant factory.
func NewEnvironmentKeyGrantResource() resource.Resource { return &environmentKeyGrantResource{} }

type environmentKeyGrantResource struct{ data *providerData }

type environmentKeyGrantModel struct {
	ID            types.String `tfsdk:"id"`
	EnvironmentID types.String `tfsdk:"environment_id"`
	PrincipalType types.String `tfsdk:"principal_type"`
	PrincipalID   types.String `tfsdk:"principal_id"`
	CreatedAt     types.String `tfsdk:"created_at"`
}

func (r *environmentKeyGrantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_key_grant"
}

func (r *environmentKeyGrantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Grants a principal (user or service token) the ability to decrypt an " +
			"environment. The provider unwraps its own DEK grant and re-wraps it to the recipient's " +
			"public key — so the configured token must already hold a grant on the environment " +
			"(it does if it created the environment).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-generated grant id (`ek_…`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The environment (`env_…`) to grant access to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"principal_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The principal kind: `user` or `service_token`.",
				Validators:          []validator.String{stringvalidator.OneOf("user", "service_token")},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"principal_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The principal's id (a user id or `skt_…` token id).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (ISO 8601).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *environmentKeyGrantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = configureResource(req, resp)
}

// recipientPublicKey looks up the JWK to wrap the DEK to, by principal kind.
func (r *environmentKeyGrantResource) recipientPublicKey(ctx context.Context, principalType, principalID string) (string, error) {
	switch principalType {
	case "service_token":
		tok, err := r.data.client.GetToken(ctx, r.data.orgID, principalID)
		if err != nil {
			return "", err
		}
		if tok == nil {
			return "", fmt.Errorf("service token %q not found in org", principalID)
		}
		return tok.PublicKeyJwk, nil
	case "user":
		members, err := r.data.client.ListMembers(ctx, r.data.orgID)
		if err != nil {
			return "", err
		}
		for _, m := range members {
			if m.UserID == principalID {
				if m.PublicKeyJwk == nil {
					return "", fmt.Errorf("user %q has not completed key setup, so cannot receive a grant", principalID)
				}
				return *m.PublicKeyJwk, nil
			}
		}
		return "", fmt.Errorf("user %q not found in org", principalID)
	default:
		return "", fmt.Errorf("unknown principal type %q", principalType)
	}
}

func (r *environmentKeyGrantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentKeyGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID := plan.EnvironmentID.ValueString()

	recipientJWK, err := r.recipientPublicKey(ctx, plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error resolving grant recipient", err.Error())
		return
	}
	// Unwrap our own DEK for this env, then re-wrap it to the recipient.
	ownWrapped, err := r.data.client.GetMyEnvKey(ctx, r.data.orgID, envID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error fetching the environment key",
			"The configured token must already hold a grant on this environment to grant others. "+err.Error(),
		)
		return
	}
	dek, err := crypto.UnwrapDEK(ownWrapped, r.data.tokenPriv)
	if err != nil {
		resp.Diagnostics.AddError("Error unwrapping the environment key", err.Error())
		return
	}
	wrapped, err := crypto.WrapDEK(dek, recipientJWK)
	if err != nil {
		resp.Diagnostics.AddError("Error wrapping the DEK for the recipient", err.Error())
		return
	}
	grant, err := r.data.client.GrantEnvKey(ctx, r.data.orgID, envID,
		plan.PrincipalType.ValueString(), plan.PrincipalID.ValueString(), wrapped)
	if err != nil {
		resp.Diagnostics.AddError("Error creating key grant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, environmentKeyGrantModel{
		ID:            types.StringValue(grant.ID),
		EnvironmentID: types.StringValue(grant.EnvironmentID),
		PrincipalType: types.StringValue(grant.PrincipalType),
		PrincipalID:   types.StringValue(grant.PrincipalID),
		CreatedAt:     types.StringValue(grant.CreatedAt),
	})...)
}

func (r *environmentKeyGrantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentKeyGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	grants, err := r.data.client.ListEnvKeys(ctx, r.data.orgID, state.EnvironmentID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading key grants", err.Error())
		return
	}
	for _, g := range grants {
		if g.ID == state.ID.ValueString() {
			resp.Diagnostics.Append(resp.State.Set(ctx, environmentKeyGrantModel{
				ID:            types.StringValue(g.ID),
				EnvironmentID: types.StringValue(g.EnvironmentID),
				PrincipalType: types.StringValue(g.PrincipalType),
				PrincipalID:   types.StringValue(g.PrincipalID),
				CreatedAt:     types.StringValue(g.CreatedAt),
			})...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

// Update never does real work: every configurable attribute is RequiresReplace,
// so a change recreates the grant. The method exists to satisfy the interface.
func (r *environmentKeyGrantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentKeyGrantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentKeyGrantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentKeyGrantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.data.client.RevokeEnvKey(ctx, r.data.orgID, state.EnvironmentID.ValueString(), state.ID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error revoking key grant", err.Error())
	}
}

func (r *environmentKeyGrantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import id",
			"Expected `<environment_id>:<grant_id>` (e.g. `env_abc:ek_xyz`).",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("environment_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
