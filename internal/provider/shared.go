package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// configureResource extracts the shared providerData from a resource Configure
// request. Returns nil when the provider has not been configured yet (a normal
// transient state) or on a type mismatch (a provider bug, reported as a
// diagnostic) — callers must treat nil as "skip".
func configureResource(req resource.ConfigureRequest, resp *resource.ConfigureResponse) *providerData {
	if req.ProviderData == nil {
		return nil
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			"Expected *providerData. This is a bug in the provider; please report it.",
		)
		return nil
	}
	return data
}

// optionalString converts a possibly-null/unknown types.String to a *string
// (nil when null or unknown) for optional request fields.
func optionalString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// stringOrNull converts a *string to a types.String (null when nil).
func stringOrNull(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

// configureDataSource is the datasource counterpart of configureResource.
func configureDataSource(req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) *providerData {
	if req.ProviderData == nil {
		return nil
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			"Expected *providerData. This is a bug in the provider; please report it.",
		)
		return nil
	}
	return data
}

// configureEphemeral is the ephemeral-resource counterpart of configureResource.
func configureEphemeral(req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) *providerData {
	if req.ProviderData == nil {
		return nil
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			"Expected *providerData. This is a bug in the provider; please report it.",
		)
		return nil
	}
	return data
}

// requireExactlyOneLookupKey enforces the "look it up by exactly one of these"
// rule the data sources share. Only null-ness is checked: an argument that
// references another resource's computed attribute is Unknown at plan time,
// which still counts as set.
func requireExactlyOneLookupKey(diags *diag.Diagnostics, what string, id, alternate types.String) {
	byID := !id.IsNull()
	byAlternate := !alternate.IsNull()
	if byID == byAlternate {
		diags.AddError(
			fmt.Sprintf("Ambiguous %s lookup", what),
			fmt.Sprintf("Set exactly one of `id` or the lookup attribute to select an %s.", what),
		)
	}
}
