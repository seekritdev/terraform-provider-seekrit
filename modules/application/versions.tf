# A module declares which providers it needs, never how to configure them —
# credentials come from the root module's provider block. `~>` on the minor
# version because the provider is pre-1.0: a minor bump may add resources this
# module does not know about, but will not move an attribute under it.
terraform {
  required_version = ">= 1.11.0"

  required_providers {
    seekrit = {
      source  = "seekritdev/seekrit"
      version = "~> 0.1"
    }
  }
}
