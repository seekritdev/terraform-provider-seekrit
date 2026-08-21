variable "name" {
  description = "Display name for the application. Editable in place."
  type        = string
}

variable "slug" {
  description = <<-EOT
    URL-safe identifier, unique within the organization. Immutable: changing it
    replaces the application, which destroys its environments and every secret
    in them.
  EOT
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]+(?:-[a-z0-9]+)*$", var.slug))
    error_message = "slug must be lowercase alphanumeric with single hyphens (`my-app`)."
  }
}

variable "environments" {
  description = <<-EOT
    The application's environments, keyed by slug. `name` defaults to a
    title-cased slug. `groups` is an ordered list of group ids composed into the
    environment — later entries win, matching the provider's `position`
    semantics.

        environments = {
          production = { name = "Production", groups = [module.shared.group_id] }
          staging    = {}
        }
  EOT
  type = map(object({
    name   = optional(string)
    groups = optional(list(string), [])
  }))

  validation {
    condition     = alltrue([for slug in keys(var.environments) : can(regex("^[a-z0-9]+(?:-[a-z0-9]+)*$", slug))])
    error_message = "Environment keys are slugs: lowercase alphanumeric with single hyphens."
  }
}

variable "runtime_tokens" {
  description = <<-EOT
    Service tokens to mint, keyed by a short label. Each is bound to one of this
    module's environments (by slug) and granted the key needed to decrypt it —
    the two-resource pairing every runtime credential needs.

        runtime_tokens = {
          ci = { environment = "production", expires_at = "2027-01-01T00:00:00Z" }
        }

    The minted secrets come back in the `tokens` output, and therefore live in
    Terraform state. Use an encrypted state backend.
  EOT
  type = map(object({
    environment = string
    role        = optional(string, "member")
    expires_at  = optional(string)
  }))
  default = {}

  validation {
    condition     = alltrue([for t in values(var.runtime_tokens) : contains(["member", "admin"], t.role)])
    error_message = "Token role must be `member` or `admin`."
  }
}

variable "grants" {
  description = <<-EOT
    Extra principals granted decrypt access, keyed by environment slug. Use it
    for the humans and for tokens minted elsewhere; `runtime_tokens` already
    grants the tokens it creates.

        grants = {
          production = [{ principal_type = "user", principal_id = "usr_…" }]
        }
  EOT
  type = map(list(object({
    principal_type = string
    principal_id   = string
  })))
  default = {}

  validation {
    condition = alltrue(flatten([
      for env, list in var.grants : [
        for g in list : contains(["user", "service_token"], g.principal_type)
      ]
    ]))
    error_message = "principal_type must be `user` or `service_token`."
  }
}

variable "token_name_prefix" {
  description = <<-EOT
    Prefix for minted token names, which are `<prefix><label>`. Defaults to the
    application slug plus a hyphen, so the token list stays readable when several
    applications each mint a `ci` token.
  EOT
  type        = string
  default     = null
}
