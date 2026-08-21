# No arguments: a service token belongs to exactly one org, so there is nothing
# to look up. Useful as a guard that a configuration is pointed where you think.
data "seekrit_organization" "current" {}

check "provider_credential_is_admin" {
  assert {
    condition     = data.seekrit_organization.current.role == "admin"
    error_message = "SEEKRIT_TOKEN is a member token; creating applications and minting tokens needs an admin token."
  }
}
