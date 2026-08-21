# An environment is owned by exactly one of an application or a group.
resource "seekrit_environment" "production" {
  application_id = seekrit_application.web.id
  name           = "Production"
  slug           = "production"
}

# The group-side environment. Composition matches on slug, so the group
# environment a `production` app environment inherits is the one also called
# `production`.
resource "seekrit_environment" "observability_production" {
  group_id = seekrit_group.observability.id
  name     = "Production"
  slug     = "production"
}
