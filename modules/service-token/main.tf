# One service token plus the key grants that make it useful.
#
# The pairing is the point. A token is a credential; a grant is the wrapped DEK
# that lets that credential decrypt one environment. Minting a token without a
# grant produces something that authenticates and then resolves nothing, which
# reads as a broken deploy rather than a missing grant. Keeping both in one
# module means the two cannot drift apart, and destroying the module revokes the
# credential and its access together.

resource "seekrit_service_token" "this" {
  name           = var.name
  role           = var.role
  environment_id = var.environment_id
  expires_at     = var.expires_at
}

resource "seekrit_environment_key_grant" "this" {
  for_each = toset(var.environment_ids)

  environment_id = each.value
  principal_type = "service_token"
  principal_id   = seekrit_service_token.this.id
}
