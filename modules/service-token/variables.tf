variable "name" {
  description = "Display name for the token. Editable in place; everything else about a token is immutable."
  type        = string
}

variable "environment_id" {
  description = <<-EOT
    The application environment (`env_…`) to bind the token to, so it can call
    `GET /v1/resolve` without naming an environment. Leave null for an org-level
    token — an admin provisioning credential, or a reader that is granted
    several environments explicitly.
  EOT
  type        = string
  default     = null
}

variable "role" {
  description = <<-EOT
    Org capability. `member` for a runtime credential; `admin` only for a
    provisioning token (an admin token can mint tokens and grant keys, which is
    what this provider itself needs).
  EOT
  type        = string
  default     = "member"

  validation {
    condition     = contains(["member", "admin"], var.role)
    error_message = "role must be `member` or `admin`."
  }
}

variable "expires_at" {
  description = <<-EOT
    Optional expiry, RFC 3339 (`2027-01-01T00:00:00Z`). Immutable: moving it
    mints a new token, because the old secret string cannot be re-issued. To
    rotate on a schedule, drive this from a `time_rotating` resource and let the
    replacement happen.
  EOT
  type        = string
  default     = null
}

variable "environment_ids" {
  description = <<-EOT
    Environments this token may decrypt. Include `environment_id` here too if the
    token should read the environment it is bound to — binding controls which
    environment `resolve` defaults to, the grant controls whether it can decrypt
    anything, and they are independent.

    The provider re-wraps each environment's DEK to this token's public key, so
    the configured provider credential must itself hold a grant on every
    environment listed.
  EOT
  type        = list(string)
  default     = []
}
