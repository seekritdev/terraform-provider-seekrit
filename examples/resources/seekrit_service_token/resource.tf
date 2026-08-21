# A runtime credential for one environment. Binding it to the environment lets
# `GET /v1/resolve` work without naming one; the grant below is what lets it
# decrypt. Both are needed — a bound token with no grant authenticates and then
# resolves nothing.
resource "seekrit_service_token" "ci" {
  name           = "ci-deploy"
  role           = "member"
  environment_id = seekrit_environment.production.id
  expires_at     = "2027-01-01T00:00:00Z"
}

resource "seekrit_environment_key_grant" "ci_production" {
  environment_id = seekrit_environment.production.id
  principal_type = "service_token"
  principal_id   = seekrit_service_token.ci.id
}

# The token string is returned once, at creation. It is stored in Terraform
# state (sensitive) — unavoidable for a resource that mints a credential, the
# same as `aws_iam_access_key.secret`. Use an encrypted state backend.
output "ci_token" {
  value     = seekrit_service_token.ci.token
  sensitive = true
}
