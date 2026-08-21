# An application, its environments, the groups each environment composes, and
# the runtime tokens that read them — the four-resource shape every seekrit
# application repeats, expressed once.
#
# What this module deliberately does NOT do is manage secret values. Those are
# `seekrit_secret` resources in the calling configuration, because their values
# must arrive through an ephemeral variable or a write-only argument and passing
# them down through module inputs adds a layer with nothing to gain. See the
# `secrets` example.

locals {
  # A slug is a fine default display name; "production" reads as "Production".
  environments = {
    for slug, env in var.environments : slug => {
      name   = coalesce(env.name, title(replace(slug, "-", " ")))
      groups = env.groups
    }
  }

  # One entry per (environment, group) pair, with position taken from the list
  # order so precedence is whatever the caller wrote.
  compositions = merge([
    for slug, env in local.environments : {
      for index, group_id in env.groups :
      "${slug}/${group_id}" => {
        environment = slug
        group_id    = group_id
        position    = index
      }
    }
  ]...)

  token_prefix = coalesce(var.token_name_prefix, "${var.slug}-")

  # Flatten grants into one entry per grant so each is its own resource
  # instance — a removed grant then destroys just that grant.
  extra_grants = merge([
    for slug, list in var.grants : {
      for g in list :
      "${slug}/${g.principal_type}/${g.principal_id}" => merge(g, { environment = slug })
    }
  ]...)
}

resource "seekrit_application" "this" {
  name = var.name
  slug = var.slug
}

resource "seekrit_environment" "this" {
  for_each = local.environments

  application_id = seekrit_application.this.id
  name           = each.value.name
  slug           = each.key
}

resource "seekrit_environment_group" "this" {
  for_each = local.compositions

  environment_id = seekrit_environment.this[each.value.environment].id
  group_id       = each.value.group_id
  position       = each.value.position
}

resource "seekrit_service_token" "this" {
  for_each = var.runtime_tokens

  name           = "${local.token_prefix}${each.key}"
  role           = each.value.role
  environment_id = seekrit_environment.this[each.value.environment].id
  expires_at     = each.value.expires_at
}

# A bound token can authenticate; it still needs the wrapped DEK to decrypt.
# Minting one without this grant produces a credential that resolves nothing,
# which is the single most common way to get this wrong by hand.
resource "seekrit_environment_key_grant" "runtime" {
  for_each = var.runtime_tokens

  environment_id = seekrit_environment.this[each.value.environment].id
  principal_type = "service_token"
  principal_id   = seekrit_service_token.this[each.key].id
}

resource "seekrit_environment_key_grant" "extra" {
  for_each = local.extra_grants

  environment_id = seekrit_environment.this[each.value.environment].id
  principal_type = each.value.principal_type
  principal_id   = each.value.principal_id
}
