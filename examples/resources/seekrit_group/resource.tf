# A group is a reusable secret bag: store a shared value once and compose it
# into every application that needs it, rather than copying it per application.
resource "seekrit_group" "observability" {
  name = "Observability"
  slug = "observability"
}
