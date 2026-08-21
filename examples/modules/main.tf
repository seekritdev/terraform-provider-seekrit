# The three modules, minimally. Sources are relative here because they live in
# this repository; from outside, source them from the mirror:
#
#   source = "git::https://github.com/seekritdev/terraform-provider-seekrit.git//modules/application?ref=v0.1.0"
#
# Always pin `ref` to a tag. An unpinned module source means a `terraform init`
# can change what your infrastructure is.

module "shared" {
  source = "../../modules/group"

  name         = "Shared"
  slug         = "shared"
  environments = { production = {} }
}

module "web" {
  source = "../../modules/application"

  name = "Web"
  slug = "web"

  environments = {
    production = { groups = [module.shared.group_id] }
  }

  runtime_tokens = {
    ci = { environment = "production", expires_at = "2027-01-01T00:00:00Z" }
  }

  grants = {
    production = [{ principal_type = "user", principal_id = "usr_5tBnKq8WvZ2" }]
  }
}

module "readonly" {
  source = "../../modules/service-token"

  name            = "audit-reader"
  environment_ids = [module.web.environment_ids["production"]]
}
