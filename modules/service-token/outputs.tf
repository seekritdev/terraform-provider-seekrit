output "token_id" {
  description = "The public token id (`skt_…`). Safe to log, and what audit rows reference."
  value       = seekrit_service_token.this.id
}

output "token" {
  description = <<-EOT
    The full secret token string, returned only at creation and stored in
    Terraform state (unavoidable for a resource that mints a credential — the
    same is true of `aws_iam_access_key.secret`). Use an encrypted state backend.
  EOT
  value       = seekrit_service_token.this.token
  sensitive   = true
}

output "public_key_jwk" {
  description = "The token's public key, in case something outside this module needs to wrap a key to it."
  value       = seekrit_service_token.this.public_key_jwk
}

output "granted_environment_ids" {
  description = "The environments this token can decrypt."
  value       = [for grant in seekrit_environment_key_grant.this : grant.environment_id]
}
