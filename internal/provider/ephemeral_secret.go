// Ephemeral resources are how this provider hands out secret values.
//
// Terraform never persists an ephemeral result: it is not written to state, not
// written to the plan file, and is re-fetched on each operation that needs it.
// That is the only shape in which a zero-knowledge secrets manager can safely
// expose plaintext to Terraform — and it is why there is no
// `data "seekrit_secret"`, which would land the value in state.
//
// The decryption happens here, in the provider process, from the environment DEK
// the configured token can unwrap. The API is not involved beyond serving
// ciphertext.
package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
)

var (
	_ ephemeral.EphemeralResource              = &secretEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &secretEphemeralResource{}
	_ ephemeral.EphemeralResource              = &secretsEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &secretsEphemeralResource{}
)

// ── one secret ──────────────────────────────────────────────────────────────

// NewSecretEphemeralResource is the seekrit_secret ephemeral resource factory.
func NewSecretEphemeralResource() ephemeral.EphemeralResource {
	return &secretEphemeralResource{}
}

type secretEphemeralResource struct{ data *providerData }

type secretEphemeralModel struct {
	EnvironmentID types.String `tfsdk:"environment_id"`
	Name          types.String `tfsdk:"name"`
	Value         types.String `tfsdk:"value"`
	Version       types.Int64  `tfsdk:"version"`
}

func (e *secretEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secret"
}

func (e *secretEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads and decrypts one secret. The value is **ephemeral**: Terraform " +
			"keeps it in memory for the operation that uses it and writes it neither to state nor " +
			"to the plan file. Feed it to another provider's write-only argument or to a " +
			"provider block — Terraform will refuse to put it anywhere it would be persisted.\n\n" +
			"The configured service token must hold a key grant on the environment.",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The environment (`env_…`) holding the secret.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The secret name (`DATABASE_URL`).",
			},
			"value": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The decrypted secret value.",
			},
			"version": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "The secret's server-side version counter.",
			},
		},
	}
}

func (e *secretEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	e.data = configureEphemeral(req, resp)
}

func (e *secretEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg secretEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID := cfg.EnvironmentID.ValueString()
	name := cfg.Name.ValueString()

	secret, err := e.data.client.GetSecret(ctx, e.data.orgID, envID, name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading secret", err.Error())
		return
	}
	if secret == nil {
		resp.Diagnostics.AddError("Secret not found",
			fmt.Sprintf("No secret named %q in environment %s.", name, envID))
		return
	}
	dek, err := e.data.envDEK(ctx, envID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving the environment key", err.Error())
		return
	}
	value, err := crypto.DecryptSecret(dek, secret.Ciphertext, crypto.SecretAAD(envID, name))
	if err != nil {
		resp.Diagnostics.AddError("Error decrypting secret", err.Error())
		return
	}
	cfg.Value = types.StringValue(value)
	cfg.Version = types.Int64Value(secret.Version)
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}

// ── every secret in an environment ──────────────────────────────────────────

// NewSecretsEphemeralResource is the seekrit_secrets ephemeral resource factory.
func NewSecretsEphemeralResource() ephemeral.EphemeralResource {
	return &secretsEphemeralResource{}
}

type secretsEphemeralResource struct{ data *providerData }

type secretsEphemeralModel struct {
	EnvironmentID types.String `tfsdk:"environment_id"`
	Values        types.Map    `tfsdk:"values"`
	Names         types.List   `tfsdk:"names"`
}

func (e *secretsEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_secrets"
}

func (e *secretsEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads and decrypts **every** secret in an environment as a map — the " +
			"Terraform equivalent of `seekrit run`, for wiring a whole environment into a " +
			"container definition's write-only environment block. Ephemeral: nothing is written " +
			"to state or to the plan file.\n\n" +
			"Note this returns the environment's **own** secrets. Group composition is applied by " +
			"`GET /v1/resolve` at runtime, not here, so a value inherited from a composed group " +
			"will not appear.",
		Attributes: map[string]schema.Attribute{
			"environment_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The environment (`env_…`) to read.",
			},
			"values": schema.MapAttribute{
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Secret name → decrypted value.",
			},
			"names": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				MarkdownDescription: "The secret names, sorted. Not sensitive — useful for " +
					"`for_each` and for asserting an environment has what you expect without " +
					"touching the values.",
			},
		},
	}
}

func (e *secretsEphemeralResource) Configure(_ context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	e.data = configureEphemeral(req, resp)
}

func (e *secretsEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var cfg secretsEphemeralModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	envID := cfg.EnvironmentID.ValueString()

	secrets, err := e.data.client.ListSecrets(ctx, e.data.orgID, envID)
	if err != nil {
		resp.Diagnostics.AddError("Error listing secrets", err.Error())
		return
	}
	dek, err := e.data.envDEK(ctx, envID)
	if err != nil {
		resp.Diagnostics.AddError("Error resolving the environment key", err.Error())
		return
	}

	values := make(map[string]string, len(secrets))
	names := make([]string, 0, len(secrets))
	for _, s := range secrets {
		value, err := crypto.DecryptSecret(dek, s.Ciphertext, crypto.SecretAAD(envID, s.Name))
		if err != nil {
			// Name the secret, never the ciphertext or any partial plaintext.
			resp.Diagnostics.AddError("Error decrypting secret",
				fmt.Sprintf("%s: %s", s.Name, err.Error()))
			return
		}
		values[s.Name] = value
		names = append(names, s.Name)
	}
	sort.Strings(names)

	valueMap, diags := types.MapValueFrom(ctx, types.StringType, values)
	resp.Diagnostics.Append(diags...)
	nameList, diags := types.ListValueFrom(ctx, types.StringType, names)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	cfg.Values = valueMap
	cfg.Names = nameList
	resp.Diagnostics.Append(resp.Result.Set(ctx, cfg)...)
}
