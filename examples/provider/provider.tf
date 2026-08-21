terraform {
  # 1.11 for write-only arguments (`seekrit_secret.value_wo`); 1.10 introduced
  # the ephemeral resources this provider uses to hand out secret values.
  required_version = ">= 1.11.0"

  required_providers {
    seekrit = {
      source  = "seekritdev/seekrit"
      version = "~> 0.1"
    }
  }
}

# Every argument falls back to an environment variable, so CI can configure the
# provider without any of this appearing in the configuration.
provider "seekrit" {
  endpoint = "https://api.seekrit.dev" # or SEEKRIT_API_URL
  org_id   = "org_your_org_id"         # or SEEKRIT_ORG
  token    = var.seekrit_token         # or SEEKRIT_TOKEN
}

# An admin service token: `seekrit token create --admin --name terraform`. It must
# be admin because this provider creates applications, mints tokens, grants keys.
#
# The token string embeds its own private key. That is what lets the provider do
# the client-side crypto — wrapping environment DEKs, re-wrapping them for
# grants, encrypting secret values — without seekrit ever holding a key.
variable "seekrit_token" {
  type      = string
  sensitive = true

  validation {
    condition     = startswith(var.seekrit_token, "skt_")
    error_message = "Expected a service token (`skt_…`). User credentials (`skc_…`) cannot mint tokens or grant keys."
  }
}
