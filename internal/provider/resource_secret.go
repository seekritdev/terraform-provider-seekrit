package provider

import (
	"context"
	"fmt"
	"strings"

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
	_ resource.Resource                = &secretResource{}
	_ resource.ResourceWithConfigure   = &secretResource{}
	_ resource.ResourceWithImportState = &secretResource{}
)

// NewSecretResource is the seekrit_secret resource factory.
func NewSecretResource() resource.Resource { return &secretResource{} }

type secretResource struct{ data *providerData }

type secretModel struct {
	ID             types.String `tfsdk:"id"`
	EnvironmentID  types.String `tfsdk:"environment_id"`
	Name           types.String `tfsdk:"name"`
	Value          types.String `tfsdk:"value_wo"`
	ValueWOVersion types.Int64  `tfsdk:"value_wo_version"`
	Version        types.Int64  `tfsdk:"version"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
}

func (r *secretResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (r *secretResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A secret in an environment.\n\n" +
			"The value is a **write-only argument** (Terraform 1.11+): it is available to the " +
			"provider during apply and is never written to state or to a plan file. The provider " +
			"encrypts it locally under the environment's DEK — the seekrit API only ever receives " +
			"ciphertext, exactly as it does from the browser and the CLI. Nothing in this " +
			"resource's state can be used to recover the value.\n\n" +
			"The configured service token must hold a key grant on the environment " +
			"(it does automatically for environments this provider created).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Composite identifier, `<environment_id>:<name>`.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"environment_id": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The environment (`env_…`) holding the secret. Immutable — a " +
					"secret does not move between environments (its ciphertext is bound to the " +
					"environment by the AAD).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The secret name, an environment-variable identifier " +
					"(`DATABASE_URL`). Immutable — renaming destroys and recreates, which is what " +
					"a rename is: the old name stops resolving.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"value_wo": schema.StringAttribute{
				Required:  true,
				WriteOnly: true,
				Sensitive: true,
				MarkdownDescription: "The secret value. Write-only: never stored in state or a plan " +
					"file. Because Terraform cannot see a value it does not store, changing this " +
					"alone produces no diff — bump `value_wo_version` to push a new value.",
			},
			"value_wo_version": schema.Int64Attribute{
				Optional: true,
				MarkdownDescription: "Change this to tell Terraform the write-only value changed. " +
					"Any different number works; an incrementing integer is the convention. " +
					"Without it, edits to `value_wo` are silently ignored on subsequent applies.",
			},
			"version": schema.Int64Attribute{
				Computed: true,
				MarkdownDescription: "The server-side version counter, incremented on every write. " +
					"seekrit keeps every previous ciphertext, so this is also the number of writes.",
			},
			"updated_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "When the secret was last written (ISO 8601).",
			},
		},
	}
}

func (r *secretResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.data = configureResource(req, resp)
}

// write encrypts the plaintext under the environment DEK and stores the
// ciphertext. Shared by Create and Update — they differ only in diagnostics.
func (r *secretResource) write(ctx context.Context, envID, name, plaintext string) (*client.Secret, error) {
	dek, err := r.data.envDEK(ctx, envID)
	if err != nil {
		return nil, err
	}
	ciphertext, err := crypto.EncryptSecret(dek, plaintext, crypto.SecretAAD(envID, name))
	if err != nil {
		return nil, fmt.Errorf("encrypting the secret value: %w", err)
	}
	return r.data.client.PutSecret(ctx, r.data.orgID, envID, name, ciphertext)
}

func (r *secretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	// Write-only values are null in the plan by design; read them from config.
	var value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("value_wo"), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.ValueWOVersion.IsNull() {
		resp.Diagnostics.AddAttributeWarning(path.Root("value_wo_version"),
			"Secret value changes will not be detected",
			"`value_wo` is write-only, so Terraform never stores it and cannot tell when it "+
				"changes. Without `value_wo_version`, later edits to the value will plan as "+
				"no-ops. Set `value_wo_version = 1` now and increment it whenever the value changes.")
	}

	envID := plan.EnvironmentID.ValueString()
	name := plan.Name.ValueString()
	secret, err := r.write(ctx, envID, name, value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error creating secret", err.Error())
		return
	}
	plan.ID = types.StringValue(secretID(envID, name))
	plan.Version = types.Int64Value(secret.Version)
	plan.UpdatedAt = types.StringValue(secret.UpdatedAt)
	// plan.Value stays as the plan had it (null) — never persisted.
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *secretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID := state.EnvironmentID.ValueString()
	secret, err := r.data.client.GetSecret(ctx, r.data.orgID, envID, state.Name.ValueString())
	if err != nil {
		// A deleted environment takes its secrets with it.
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading secret", err.Error())
		return
	}
	if secret == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	// Only metadata is refreshed. The ciphertext is deliberately not decrypted
	// and not stored: this resource's job is to write a value, and reading one
	// back into state would defeat the write-only argument entirely.
	state.ID = types.StringValue(secretID(envID, secret.Name))
	state.Version = types.Int64Value(secret.Version)
	state.UpdatedAt = types.StringValue(secret.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *secretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan secretModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var value types.String
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("value_wo"), &value)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID := plan.EnvironmentID.ValueString()
	name := plan.Name.ValueString()
	secret, err := r.write(ctx, envID, name, value.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error updating secret", err.Error())
		return
	}
	plan.ID = types.StringValue(secretID(envID, name))
	plan.Version = types.Int64Value(secret.Version)
	plan.UpdatedAt = types.StringValue(secret.UpdatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *secretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state secretModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.data.client.DeleteSecret(ctx, r.data.orgID,
		state.EnvironmentID.ValueString(), state.Name.ValueString())
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Error deleting secret", err.Error())
	}
}

// ImportState accepts `<environment_id>:<name>`. The value is not imported —
// it cannot be, and should not be: import brings the secret under management,
// and the next apply writes whatever `value_wo` says.
func (r *secretResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	envID, name, ok := strings.Cut(req.ID, ":")
	if !ok || envID == "" || name == "" {
		resp.Diagnostics.AddError(
			"Invalid import id",
			"Expected `<environment_id>:<name>` (e.g. `env_abc:DATABASE_URL`).",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("environment_id"), envID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), secretID(envID, name))...)
}

func secretID(envID, name string) string { return envID + ":" + name }
