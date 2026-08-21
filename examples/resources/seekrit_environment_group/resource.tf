# Compose the group into the application environment. `position` is precedence:
# on a name collision, the higher position wins, and the environment's own
# secrets beat every group.
resource "seekrit_environment_group" "web_uses_observability" {
  environment_id = seekrit_environment.production.id
  group_id       = seekrit_group.observability.id
  position       = 0
}
