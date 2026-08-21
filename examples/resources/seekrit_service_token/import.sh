# Tokens import by id — but the secret string is unrecoverable, so `token` and
# `token_hash` stay null afterwards. An imported token can be renamed and
# revoked from Terraform; it cannot be handed to a consumer again.
terraform import seekrit_service_token.ci skt_4kQ9wPzMv2LbTx7RnAcYdH
