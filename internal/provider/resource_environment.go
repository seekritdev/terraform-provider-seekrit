package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
	"github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
)

var (
	_ resource.Resource                   = &environmentResource{}
	_ resource.ResourceWithConfigure      = &environmentResource{}
	_ resource.ResourceWithImportState    = &environmentResource{}
	_ resource.ResourceWithValidateConfig = &environmentResource{}
)

// NewEnvironmentResource is the seekrit_environment resource factory.
func NewEnvironmentResource() resource.Resource { return &environmentResource{} }

type environmentResource struct{ data *providerData }

type environmentModel struct {
	ID            types.String `tfsdk:"id"`
	ApplicationID types.String `tfsdk:"application_id"`
	GroupID       types.String `tfsdk:"group_id"`
	Name          types.String `tfsdk:"name"`
	Slug          types.String `tfsdk:"slug"`
	CreatedAt     types.String `tfsdk:"created_at"`
	UpdatedAt     types.String `tfsdk:"updated_at"`
}

func environmentToModel(e *client.Environment) environmentModel {
	return environmentModel{
		ID:            types.StringValue(e.ID),
		ApplicationID: stringOrNull(e.ApplicationID),
		GroupID:       stringOrNull(e.GroupID),
		Name:          types.StringValue(e.Name),
		Slug:          types.StringValue(e.Slug),
		CreatedAt:     types.StringValue(e.CreatedAt),
		UpdatedAt:     types.StringValue(e.UpdatedAt),
	}
}

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A seekrit environment, owned by exactly one of an application or a group. " +
			"On create the provider generates the environment DEK and wraps it to the configured " +
			"service token, so that token can immediately manage the environment's secrets. No " +
			"secret plaintext is involved.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Server-generated environment id (`env_…`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"application_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The owning application id. Exactly one of `application_id` or " +
					"`group_id` must be set. Immutable — changing ownership replaces the environment.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"group_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The owning group id. Exactly one of `application_id` or " +
					"`group_id` must be set. Immutable — changing ownership replaces the environment.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Human-readable environment name (editable in place).",
			},
			"slug": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "URL-safe identifier, unique per owner and the key group " +
					"composition matches on. Immutable — changing it replaces the environment.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Creation timestamp (ISO 8601).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Last-update timestamp (ISO 8601).",
			},
		},
	}
}

func (r *environmentResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg environmentModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// A value referencing another resource's computed id is Unknown (not Null)
	// at plan time — that still counts as "set", so validate on null-ness only.
	appSet := !cfg.ApplicationID.IsNull()
	grpSet := !cfg.GroupID.IsNull()
	if appSet == grpSet {
		resp.Diagnostics.AddError(
			"Invalid environment owner",
			"Exactly one of `application_id` or `group_id` must be set.",
		)
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = configureResource(req, resp)
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate a fresh DEK and wrap it to our own token so the environment is
	// born with a usable access grant for this principal. The DEK is random key
	// material, never a user secret; the wrapped form is ciphertext.
	dek, err := crypto.GenerateDEK()
	if err != nil {
		resp.Diagnostics.AddError("Error generating environment key", err.Error())
		return
	}
	wrapped, err := crypto.WrapDEK(dek, r.data.tokenPubJWK)
	if err != nil {
		resp.Diagnostics.AddError("Error wrapping environment key", err.Error())
		return
	}

	var env *client.Environment
	switch {
	case !plan.ApplicationID.IsNull():
		env, err = r.data.client.CreateAppEnv(ctx, r.data.orgID, plan.ApplicationID.ValueString(),
			plan.Name.ValueString(), plan.Slug.ValueString(), wrapped)
	default:
		env, err = r.data.client.CreateGroupEnv(ctx, r.data.orgID, plan.GroupID.ValueString(),
			plan.Name.ValueString(), plan.Slug.ValueString(), wrapped)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, environmentToModel(env))...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	env, err := r.data.client.GetEnvironment(ctx, r.data.orgID, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, environmentToModel(env))...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	env, err := r.data.client.UpdateEnvironment(ctx, r.data.orgID, plan.ID.ValueString(), plan.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, environmentToModel(env))...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteEnvironment(ctx, r.data.orgID, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting environment", err.Error())
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
