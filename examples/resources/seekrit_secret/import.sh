# Secrets import as `<environment_id>:<name>`. The value is not imported — it
# cannot be read back into a write-only argument. Import brings the secret under
# management; the next apply writes whatever `value_wo` says.
terraform import seekrit_secret.database_url env_9QpZ3vLmKd8:DATABASE_URL
