package provider_test

import (
	"context"
	"strings"
	"testing"

	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	fwephemeral "github.com/hashicorp/terraform-plugin-framework/ephemeral"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/seekritdev/terraform-provider-seekrit/internal/provider"
)

// newProvider builds the provider and asserts it exposes the ephemeral-resource
// interface (the read path for secret values lives there).
func newProvider(t *testing.T) fwprovider.ProviderWithEphemeralResources {
	t.Helper()
	p, ok := provider.New("test")().(fwprovider.ProviderWithEphemeralResources)
	if !ok {
		t.Fatal("provider does not implement ProviderWithEphemeralResources")
	}
	return p
}

// TestSchemasAreValid runs the framework's own schema implementation checks over
// every resource, data source, and ephemeral resource. These are the errors
// Terraform would otherwise raise at runtime — an attribute that is both
// Computed and WriteOnly, a plan modifier on the wrong type, a missing
// description. Cheap, offline, and it covers every schema without a live API.
func TestSchemasAreValid(t *testing.T) {
	ctx := context.Background()
	p := newProvider(t)

	var providerResp fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &providerResp)
	if diags := providerResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Errorf("provider schema: %v", diags.Errors())
	}

	for _, factory := range p.Resources(ctx) {
		r := factory()
		var meta fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "seekrit"}, &meta)
		var resp fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &resp)
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("resource %s: %v", meta.TypeName, diags.Errors())
		}
	}

	for _, factory := range p.DataSources(ctx) {
		d := factory()
		var meta fwdatasource.MetadataResponse
		d.Metadata(ctx, fwdatasource.MetadataRequest{ProviderTypeName: "seekrit"}, &meta)
		var resp fwdatasource.SchemaResponse
		d.Schema(ctx, fwdatasource.SchemaRequest{}, &resp)
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("data source %s: %v", meta.TypeName, diags.Errors())
		}
	}

	for _, factory := range p.EphemeralResources(ctx) {
		e := factory()
		var meta fwephemeral.MetadataResponse
		e.Metadata(ctx, fwephemeral.MetadataRequest{ProviderTypeName: "seekrit"}, &meta)
		var resp fwephemeral.SchemaResponse
		e.Schema(ctx, fwephemeral.SchemaRequest{}, &resp)
		if diags := resp.Schema.ValidateImplementation(ctx); diags.HasError() {
			t.Errorf("ephemeral resource %s: %v", meta.TypeName, diags.Errors())
		}
	}
}

// TestSecretValueNeverEntersState is the provider's half of the zero-knowledge
// invariant, asserted structurally rather than by inspection: any attribute
// holding a secret VALUE must be write-only (managed resources) or belong to an
// ephemeral resource (which Terraform never persists). A plain Computed or
// Optional attribute carrying plaintext would put a customer secret in
// terraform.tfstate, and this test fails if one is ever added.
//
// Minted credentials are the deliberate exception, and only on the resource that
// creates them: seekrit_service_token.token is a Terraform-created credential,
// like aws_iam_access_key.secret. It is allow-listed by name here so adding a
// second such attribute has to be an argued change.
func TestSecretValueNeverEntersState(t *testing.T) {
	ctx := context.Background()
	p := newProvider(t)

	allowedInState := map[string]bool{
		// The token this resource just minted, returned once. Documented as
		// state-resident; see resource_service_token.go.
		"seekrit_service_token.token":      true,
		"seekrit_service_token.token_hash": true,
	}

	inspected := map[string]bool{}
	for _, factory := range p.Resources(ctx) {
		r := factory()
		var meta fwresource.MetadataResponse
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "seekrit"}, &meta)
		var resp fwresource.SchemaResponse
		r.Schema(ctx, fwresource.SchemaRequest{}, &resp)

		for name, attr := range resp.Schema.Attributes {
			if !looksLikeSecretValue(name) {
				continue
			}
			qualified := meta.TypeName + "." + name
			inspected[qualified] = true
			if allowedInState[qualified] {
				continue
			}
			if !attr.IsWriteOnly() {
				t.Errorf("%s looks like it carries a secret value but is not write-only — "+
					"it would be persisted to Terraform state", qualified)
			}
			if attr.IsComputed() {
				t.Errorf("%s is Computed, so the provider would write it to state", qualified)
			}
		}
	}

	// Guard against the test passing because the heuristic stopped matching
	// anything (a renamed attribute, a rewritten schema). If these two are not
	// still in view, the check above proved nothing.
	for _, expected := range []string{"seekrit_secret.value_wo", "seekrit_service_token.token"} {
		if !inspected[expected] {
			t.Errorf("%s was not inspected — looksLikeSecretValue no longer matches it, "+
				"so this test is not actually guarding anything", expected)
		}
	}
}

// looksLikeSecretValue is a deliberately broad name heuristic: the test's job is
// to catch a plaintext attribute somebody adds without thinking about state, so
// it should over-match and force an explicit allow-list entry.
func looksLikeSecretValue(name string) bool {
	for _, needle := range []string{"value", "secret", "password", "plaintext", "token"} {
		if strings.Contains(name, needle) {
			// Version counters and hashes are metadata, not values.
			if strings.HasSuffix(name, "_version") {
				return false
			}
			return true
		}
	}
	return false
}
