# A group — a reusable, org-scoped secret bag — and its environments.
#
# Groups exist so a value that is genuinely shared (a Datadog key, a shared
# database host) is stored once and composed into each application that needs
# it, instead of copied per application and then drifting. Composition happens
# on the application side: `seekrit_environment_group`, or the `groups` input of
# the `application` module in this repository.

locals {
  environments = {
    for slug, env in var.environments : slug => {
      name = coalesce(env.name, title(replace(slug, "-", " ")))
    }
  }

  grants = merge([
    for slug, list in var.grants : {
      for g in list :
      "${slug}/${g.principal_type}/${g.principal_id}" => merge(g, { environment = slug })
    }
  ]...)
}

resource "seekrit_group" "this" {
  name = var.name
  slug = var.slug
}

resource "seekrit_environment" "this" {
  for_each = local.environments

  group_id = seekrit_group.this.id
  name     = each.value.name
  slug     = each.key
}

resource "seekrit_environment_key_grant" "this" {
  for_each = local.grants

  environment_id = seekrit_environment.this[each.value.environment].id
  principal_type = each.value.principal_type
  principal_id   = each.value.principal_id
}
