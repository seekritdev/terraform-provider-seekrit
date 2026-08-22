# A full seekrit configuration: a shared group, two applications composing it,
# an environment each, runtime credentials, and secret values.
#
# Read it as the answer to "what does managing seekrit as code actually look
# like" — the structure is Terraform's to own, and the values arrive through
# ephemeral variables so none of them lands in state.

terraform {
  required_version = ">= 1.11.0"

  required_providers {
    seekrit = {
      source  = "seekritdev/seekrit"
      version = "~> 1.0"
    }
  }
}

provider "seekrit" {
  # endpoint / org_id / token come from SEEKRIT_API_URL, SEEKRIT_ORG,
  # SEEKRIT_TOKEN. Keeping them out of the configuration is the point: the
  # credential that can mint credentials should not be in version control.
}

data "seekrit_organization" "current" {}

check "credential_can_provision" {
  assert {
    condition     = data.seekrit_organization.current.role == "admin"
    error_message = "SEEKRIT_TOKEN must be an admin service token (`seekrit token create --admin`)."
  }
}

# ── shared values, stored once ──────────────────────────────────────────────

module "observability" {
  source = "../../modules/group"

  name = "Observability"
  slug = "observability"

  # Slugs mirror the application environments that will compose this group —
  # composition matches on slug, not on id.
  environments = {
    production = {}
    staging    = {}
  }
}

# ── two applications, both composing the shared group ───────────────────────

module "web" {
  source = "../../modules/application"

  name = "Web"
  slug = "web"

  environments = {
    production = { groups = [module.observability.group_id] }
    staging    = { groups = [module.observability.group_id] }
  }

  runtime_tokens = {
    # One credential per environment, each bound and granted in one step.
    ci-production = { environment = "production" }
    ci-staging    = { environment = "staging" }
  }
}

module "api" {
  source = "../../modules/application"

  name = "API"
  slug = "api"

  environments = {
    production = { groups = [module.observability.group_id] }
  }
}

# A token that reads BOTH applications' production environments — the shape a
# deploy pipeline or an aggregating service needs, which no single bound token
# can express.
module "deployer" {
  source = "../../modules/service-token"

  name = "deployer"
  role = "member"

  environment_ids = [
    module.web.environment_ids["production"],
    module.api.environment_ids["production"],
  ]
}

# ── values ──────────────────────────────────────────────────────────────────
#
# Secrets stay in the root module: their values must come from an ephemeral
# variable, and threading those through module inputs buys nothing. Names are a
# plain variable so `for_each` has something Terraform can see at plan time.

variable "web_production_secret_names" {
  type    = set(string)
  default = ["DATABASE_URL", "STRIPE_SECRET_KEY"]
}

variable "web_production_secrets" {
  description = "Name → value. Pass with -var-file, or TF_VAR_web_production_secrets from CI."
  type        = map(string)
  sensitive   = true
  ephemeral   = true
}

resource "seekrit_secret" "web_production" {
  for_each = var.web_production_secret_names

  environment_id   = module.web.environment_ids["production"]
  name             = each.key
  value_wo         = var.web_production_secrets[each.key]
  value_wo_version = 1
}

variable "datadog_api_key" {
  type      = string
  sensitive = true
  ephemeral = true
}

# Written into the GROUP environment, so both applications inherit it and there
# is one copy to rotate.
resource "seekrit_secret" "datadog" {
  environment_id   = module.observability.environment_ids["production"]
  name             = "DATADOG_API_KEY"
  value_wo         = var.datadog_api_key
  value_wo_version = 1
}

# ── outputs ─────────────────────────────────────────────────────────────────

output "web_ci_tokens" {
  description = "Hand these to CI. Sensitive, and present in state — encrypt your backend."
  value       = module.web.tokens
  sensitive   = true
}

output "deployer_token" {
  value     = module.deployer.token
  sensitive = true
}
