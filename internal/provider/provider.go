// Package provider implements the Terraform provider for seekrit. It manages the
// control-plane resources — applications, groups, environments, composition
// links, service tokens, DEK grants, and secrets — by driving the seekrit REST
// API as an admin service-token principal.
//
// The zero-knowledge invariant holds here the same way it holds in the browser
// and the CLI: the provider is a client, and all crypto is client-side
// (internal/crypto). It generates and wraps environment DEKs, mints token
// keypairs, and encrypts secret values locally; the API receives ciphertext,
// hashes, and public keys only.
//
// Terraform adds a second constraint on top of that — state. A secret value in
// terraform.tfstate would be a plaintext copy outside seekrit, so secret values
// never enter state in either direction:
//
//   - writing goes through a write-only argument (seekrit_secret.value_wo),
//     which Terraform hands to the provider during apply and persists nowhere;
//   - reading goes through ephemeral resources (seekrit_secret,
//     seekrit_secrets), whose results Terraform keeps in memory for one
//     operation and writes to neither state nor the plan file.
//
// There is deliberately no `data "seekrit_secret"`. The one credential that does
// live in state is the token seekrit_service_token mints, which has nowhere else
// to go — see that resource's documentation. internal/provider/schema_test.go
// asserts this structurally, so a new plaintext attribute fails the build.
package provider

import (
	"context"
	"crypto/ecdh"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/seekritdev/terraform-provider-seekrit/internal/client"
	"github.com/seekritdev/terraform-provider-seekrit/internal/crypto"
)

// Ensure seekritProvider satisfies the provider interfaces it implements.
var (
	_ provider.Provider                       = &seekritProvider{}
	_ provider.ProviderWithEphemeralResources = &seekritProvider{}
)

// seekritProvider is the provider implementation.
type seekritProvider struct {
	// version is set at build time and surfaced in the provider's user agent.
	version string
}

// providerData is shared with every resource via Configure. It bundles the API
// client, the target org, and the token's crypto material (derived once from
// the configured token string).
type providerData struct {
	client      *client.Client
	orgID       string
	tokenPriv   *ecdh.PrivateKey
	tokenPubJWK string

	// deks memoizes unwrapped environment DEKs for the life of one Terraform
	// run, keyed by environment id. Writing twenty secrets into one environment
	// would otherwise fetch and unwrap the same DEK twenty times. Guarded by a
	// mutex because Terraform applies resources concurrently.
	//
	// The DEK is key material, not a secret value, and it lives only in this
	// process's memory — never in state, a plan file, or a log.
	deksMu sync.Mutex
	deks   map[string][]byte
}

// envDEK returns the unwrapped DEK for an environment, fetching and caching it
// on first use. It fails when the configured token holds no grant on the
// environment, which is the common misconfiguration: the provider can only
// encrypt for environments it can already decrypt.
func (d *providerData) envDEK(ctx context.Context, envID string) ([]byte, error) {
	d.deksMu.Lock()
	defer d.deksMu.Unlock()
	if dek, ok := d.deks[envID]; ok {
		return dek, nil
	}
	wrapped, err := d.client.GetMyEnvKey(ctx, d.orgID, envID)
	if err != nil {
		if client.IsForbidden(err) {
			return nil, fmt.Errorf(
				"the configured service token holds no key grant on environment %s, so it cannot "+
					"encrypt or decrypt its secrets. Grant it access with a "+
					"seekrit_environment_key_grant resource (or `seekrit grant`); "+
					"environments this provider created already have one", envID)
		}
		return nil, err
	}
	dek, err := crypto.UnwrapDEK(wrapped, d.tokenPriv)
	if err != nil {
		return nil, fmt.Errorf("unwrapping the DEK for environment %s: %w", envID, err)
	}
	if d.deks == nil {
		d.deks = make(map[string][]byte)
	}
	d.deks[envID] = dek
	return dek, nil
}

// seekritProviderModel maps provider block configuration.
type seekritProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	OrgID    types.String `tfsdk:"org_id"`
	Token    types.String `tfsdk:"token"`
}

// New returns a provider factory for the given build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &seekritProvider{version: version}
	}
}

func (p *seekritProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "seekrit"
	resp.Version = p.version
}

func (p *seekritProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manage seekrit — applications, environments, groups, service tokens, " +
			"key grants, and secrets — as infrastructure-as-code.\n\n" +
			"All cryptography happens in the provider process, as it does in the browser and the " +
			"CLI: the seekrit API receives ciphertext, hashes, and public keys, never a secret " +
			"value or a private key. Secret values are written through write-only arguments and " +
			"read through ephemeral resources, so no secret value is ever written to Terraform " +
			"state or to a plan file.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "Base URL of the seekrit API " +
					"(defaults to the `SEEKRIT_API_URL` environment variable).",
			},
			"org_id": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: "The organization id (`org_…`) all resources belong to " +
					"(defaults to the `SEEKRIT_ORG` environment variable).",
			},
			"token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				MarkdownDescription: "An admin service token (`skt_…`) " +
					"(defaults to the `SEEKRIT_TOKEN` environment variable). " +
					"The token string embeds its private key and is used for DEK wrapping.",
			},
		},
	}
}

func (p *seekritProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg seekritProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	endpoint := firstNonEmpty(cfg.Endpoint.ValueString(), os.Getenv("SEEKRIT_API_URL"))
	orgID := firstNonEmpty(cfg.OrgID.ValueString(), os.Getenv("SEEKRIT_ORG"))
	token := firstNonEmpty(cfg.Token.ValueString(), os.Getenv("SEEKRIT_TOKEN"))

	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(path.Root("endpoint"), "Missing API endpoint",
			"Set the provider `endpoint` or the SEEKRIT_API_URL environment variable.")
	}
	if orgID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("org_id"), "Missing organization id",
			"Set the provider `org_id` or the SEEKRIT_ORG environment variable.")
	}
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Missing service token",
			"Set the provider `token` or the SEEKRIT_TOKEN environment variable.")
	}
	if resp.Diagnostics.HasError() {
		return
	}

	// The token string carries its own private key; derive the key material the
	// resources need for DEK wrapping (environments) and grants (unwrap+rewrap).
	_, priv, err := crypto.ParseServiceToken(token)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Invalid service token", err.Error())
		return
	}
	pubJWK, err := crypto.PublicKeyJWK(priv)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Invalid service token",
			"could not derive the token's public key: "+err.Error())
		return
	}

	data := &providerData{
		client:      client.New(endpoint, token, p.version, nil),
		orgID:       orgID,
		tokenPriv:   priv,
		tokenPubJWK: pubJWK,
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *seekritProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewApplicationResource,
		NewGroupResource,
		NewEnvironmentResource,
		NewEnvironmentGroupResource,
		NewServiceTokenResource,
		NewEnvironmentKeyGrantResource,
		NewSecretResource,
	}
}

func (p *seekritProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewOrganizationDataSource,
		NewApplicationDataSource,
		NewGroupDataSource,
		NewEnvironmentDataSource,
		NewServiceTokenDataSource,
	}
}

// EphemeralResources are the read side of secret values. They exist as ephemeral
// resources rather than data sources on purpose: Terraform never writes an
// ephemeral result to state or to a plan file, so a decrypted secret can flow
// into another provider's write-only argument without being persisted anywhere.
// There is deliberately no `data "seekrit_secret"` — that would put plaintext in
// state, which is the one thing this provider must not do.
func (p *seekritProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		NewSecretEphemeralResource,
		NewSecretsEphemeralResource,
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
