# The value is a write-only argument: Terraform hands it to the provider during
# apply and stores it nowhere — not in state, not in the plan file. The provider
# encrypts it under the environment's DEK before it leaves the process, so the
# seekrit API only ever sees ciphertext.
resource "seekrit_secret" "database_url" {
  environment_id = seekrit_environment.production.id
  name           = "DATABASE_URL"

  value_wo         = var.database_url
  value_wo_version = 1 # bump this whenever the value changes
}

variable "database_url" {
  type      = string
  sensitive = true
  ephemeral = true # Terraform will refuse to persist it anywhere
}

# Writing several secrets at once. `for_each` must iterate something Terraform
# can see at plan time, so the NAMES come from a plain variable and the VALUES
# from an ephemeral map — an ephemeral value cannot drive `for_each`, because
# that would make the shape of the plan depend on data Terraform may not have.
variable "secret_names" {
  type    = set(string)
  default = ["STRIPE_SECRET_KEY", "SENTRY_DSN"]
}

variable "secret_values" {
  type      = map(string)
  sensitive = true
  ephemeral = true
}

resource "seekrit_secret" "app" {
  for_each = var.secret_names

  environment_id   = seekrit_environment.production.id
  name             = each.key
  value_wo         = var.secret_values[each.key]
  value_wo_version = 1
}
