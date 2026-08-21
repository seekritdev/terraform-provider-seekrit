data "seekrit_group" "observability" {
  slug = "observability"
}

# Compose an existing shared group into an environment Terraform owns.
resource "seekrit_environment_group" "preview_uses_observability" {
  environment_id = seekrit_environment.preview.id
  group_id       = data.seekrit_group.observability.id
}
