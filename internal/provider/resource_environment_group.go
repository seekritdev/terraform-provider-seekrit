package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
)

var (
	_ resource.Resource                = &environmentGroupResource{}
	_ resource.ResourceWithConfigure   = &environmentGroupResource{}
	_ resource.ResourceWithImportState = &environmentGroupResource{}
)

// NewEnvironmentGroupResource is the seekrit_environment_group resource factory
// (composition: which group an application environment pulls in, and at what
// precedence).
func NewEnvironmentGroupResource() resource.Resource { return &environmentGroupResource{} }

type environmentGroupResource struct{ data *providerData }

type environmentGroupModel struct {
	EnvironmentID types.String `tfsdk:"environment_id"`
	GroupID       types.String `tfsdk:"group_id"`
	Position      types.Int64  `tfsdk:"position"`
}

func (r *environmentGroupResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment_group"
}

func (r *environmentGroupResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Composes a group into an application environment. The environment must " +
			"be application-owned. Precedence is set by `position` (higher wins).",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The application environment id (`env_…`) this link belongs to.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"group_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The composed group id (`grp_…`).",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"position": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Precedence among the environment's composed groups (higher wins). " +
					"Defaults to appended-last when omitted.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *environmentGroupResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = configureResource(req, resp)
}

func (r *environmentGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var position *int64
	if !plan.Position.IsNull() && !plan.Position.IsUnknown() {
		p := plan.Position.ValueInt64()
		position = &p
	}
	ref, err := r.data.client.LinkEnvGroup(ctx, r.data.orgID,
		plan.EnvironmentID.ValueString(), plan.GroupID.ValueString(), position)
	if err != nil {
		resp.Diagnostics.AddError("Error linking group to environment", err.Error())
		return
	}
	plan.Position = types.Int64Value(ref.Position)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	groups, err := r.data.client.ListEnvGroups(ctx, r.data.orgID, state.EnvironmentID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading environment groups", err.Error())
		return
	}
	for _, g := range groups {
		if g.GroupID == state.GroupID.ValueString() {
			state.Position = types.Int64Value(g.Position)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	// The link no longer exists.
	resp.State.RemoveResource(ctx)
}

func (r *environmentGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Only position is mutable; re-linking upserts it (idempotent on env+group).
	var position *int64
	if !plan.Position.IsNull() && !plan.Position.IsUnknown() {
		p := plan.Position.ValueInt64()
		position = &p
	}
	ref, err := r.data.client.LinkEnvGroup(ctx, r.data.orgID,
		plan.EnvironmentID.ValueString(), plan.GroupID.ValueString(), position)
	if err != nil {
		resp.Diagnostics.AddError("Error updating group composition", err.Error())
		return
	}
	plan.Position = types.Int64Value(ref.Position)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.data.client.UnlinkEnvGroup(ctx, r.data.orgID,
		state.EnvironmentID.ValueString(), state.GroupID.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error unlinking group from environment", err.Error())
	}
}

func (r *environmentGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Invalid import id",
			"Expected `<environment_id>:<group_id>` (e.g. `env_abc:grp_xyz`).",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("environment_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("group_id"), parts[1])...)
}
