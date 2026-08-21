# The whole environment at once — the Terraform equivalent of `seekrit run`, for
# wiring a complete environment into a task definition or a container's
# write-only environment block.
ephemeral "seekrit_secrets" "production" {
  environment_id = data.seekrit_environment.production.id
}

# Copy an environment's secrets into another environment. The NAMES come from a
# plain variable, not from `names`: every attribute of an ephemeral resource is
# an ephemeral value, and `for_each` cannot take one — it decides resource
# addresses, which have to survive into state. `values` supplies the plaintext,
# which is exactly what should not.
variable "mirrored_names" {
  type    = set(string)
  default = ["STRIPE_SECRET_KEY", "SENTRY_DSN"]
}

resource "seekrit_secret" "mirrored_to_staging" {
  for_each = var.mirrored_names

  environment_id   = data.seekrit_environment.staging.id
  name             = each.key
  value_wo         = ephemeral.seekrit_secrets.production.values[each.key]
  value_wo_version = 1
}

# `names` is the non-sensitive companion to `values`: it says which secrets an
# environment holds without exposing any of them, which is what you want in a
# lifecycle precondition or anywhere you are checking shape rather than content.

# Note: this returns the environment's OWN secrets. Values inherited from
# composed groups are merged by `GET /v1/resolve` at runtime, not here — so a
# secret that lives in a group will not appear. Read the group's environment
# directly if you need it.
