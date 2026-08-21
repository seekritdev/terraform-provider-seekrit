resource "seekrit_application" "web" {
  name = "Web"
  slug = "web" # immutable: changing it replaces the application
}
