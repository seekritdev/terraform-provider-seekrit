# Ephemeral resources are how this provider hands out secret values. Terraform
# keeps an ephemeral result in memory for the operation that needs it and writes
# it neither to state nor to the plan file, then discards it. That is the only
# shape in which a zero-knowledge secrets manager can safely feed Terraform —
# which is why there is no `data "seekrit_secret"`.
ephemeral "seekrit_secret" "db_password" {
  environment_id = data.seekrit_environment.production.id
  name           = "DATABASE_PASSWORD"
}

# Into another provider's write-only argument. Terraform enforces the pairing:
# an ephemeral value may only flow somewhere that will not persist it, so this
# compiles and `password = …` would not.
resource "aws_db_instance" "app" {
  identifier          = "app"
  engine              = "postgres"
  instance_class      = "db.t4g.micro"
  allocated_storage   = 20
  username            = "app"
  password_wo         = ephemeral.seekrit_secret.db_password.value
  password_wo_version = 1
  skip_final_snapshot = true
}

# Into a provider block, which is evaluated per-operation and never stored.
ephemeral "seekrit_secret" "cloudflare_token" {
  environment_id = data.seekrit_environment.production.id
  name           = "CLOUDFLARE_API_TOKEN"
}

provider "cloudflare" {
  api_token = ephemeral.seekrit_secret.cloudflare_token.value
}
