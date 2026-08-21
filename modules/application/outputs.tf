output "application_id" {
  description = "The application id (`app_…`)."
  value       = seekrit_application.this.id
}

output "slug" {
  description = "The application slug, echoed for composing names downstream."
  value       = seekrit_application.this.slug
}

output "environment_ids" {
  description = "Environment slug → id. This is what `seekrit_secret` and grants in the calling configuration key off."
  value       = { for slug, env in seekrit_environment.this : slug => env.id }
}

output "token_ids" {
  description = "Token label → public token id (`skt_…`). Not sensitive."
  value       = { for label, token in seekrit_service_token.this : label => token.id }
}

output "tokens" {
  description = <<-EOT
    Token label → the full secret token string, returned only at creation.

    Sensitive, and state-resident: a resource that mints a credential has nowhere
    else to keep it (the same is true of `aws_iam_access_key.secret`). Use an
    encrypted state backend, and prefer feeding these straight into the consumer
    (a CI secret, a container definition) over printing them.
  EOT
  value       = { for label, token in seekrit_service_token.this : label => token.token }
  sensitive   = true
}
