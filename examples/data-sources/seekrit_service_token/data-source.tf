# The secret string is NOT available here — it exists only in the response to
# the call that created the token. What this is for is `public_key_jwk`: the key
# a DEK grant is wrapped to. Use it to grant a token minted by the CLI or the
# dashboard access to a Terraform-managed environment.
data "seekrit_service_token" "existing_ci" {
  name = "ci-deploy"
}

resource "seekrit_environment_key_grant" "existing_ci_preview" {
  environment_id = seekrit_environment.preview.id
  principal_type = "service_token"
  principal_id   = data.seekrit_service_token.existing_ci.id
}
