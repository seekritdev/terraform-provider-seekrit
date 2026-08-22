# A module declares which providers it needs, never how to configure them —
# credentials come from the root module's provider block.
#
# `~> 1.0` is the minor range: >= 1.0.0, < 2.0.0. Now that the provider is past
# 1.0, semver means what it says — a minor bump only adds — so a module should
# accept them rather than pin a patch range and need editing on every release.
# The major boundary is the one that matters, and it is the one this excludes.
terraform {
  required_version = ">= 1.11.0"

  required_providers {
    seekrit = {
      source  = "seekritdev/seekrit"
      version = "~> 1.0"
    }
  }
}
