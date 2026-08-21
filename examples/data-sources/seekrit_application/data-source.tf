# Attach Terraform-managed environments to an application someone created in the
# dashboard, without importing the application itself.
data "seekrit_application" "web" {
  slug = "web"
}

resource "seekrit_environment" "preview" {
  application_id = data.seekrit_application.web.id
  name           = "Preview"
  slug           = "preview"
}
