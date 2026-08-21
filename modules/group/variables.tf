variable "name" {
  description = "Display name for the group. Editable in place."
  type        = string
}

variable "slug" {
  description = <<-EOT
    URL-safe identifier, unique within the organization. Immutable: changing it
    replaces the group and everything in it.
  EOT
  type        = string

  validation {
    condition     = can(regex("^[a-z0-9]+(?:-[a-z0-9]+)*$", var.slug))
    error_message = "slug must be lowercase alphanumeric with single hyphens (`shared-infra`)."
  }
}

variable "environments" {
  description = <<-EOT
    The group's environments, keyed by slug. Composition matches on slug: a
    group environment named `production` is the one an application's
    `production` environment inherits, so the keys here should mirror the
    application environments that will compose this group.

        environments = { production = {}, staging = { name = "Staging" } }
  EOT
  type = map(object({
    name = optional(string)
  }))

  validation {
    condition     = alltrue([for slug in keys(var.environments) : can(regex("^[a-z0-9]+(?:-[a-z0-9]+)*$", slug))])
    error_message = "Environment keys are slugs: lowercase alphanumeric with single hyphens."
  }
}

variable "grants" {
  description = <<-EOT
    Principals granted decrypt access, keyed by environment slug.

        grants = {
          production = [{ principal_type = "user", principal_id = "usr_…" }]
        }

    Group environments hold no tokens of their own — a service token binds to an
    application environment — so grants here are for people and for tokens that
    read the group directly.
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
