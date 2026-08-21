# Grant a teammate decrypt access to an environment. The provider fetches its
# own wrapped DEK for the environment, unwraps it locally, and re-wraps it to
# the recipient's public key — so the plaintext DEK exists only inside the
# provider process, and the API stores nothing it can read.
#
# The recipient must already have keys set up (a user completes key setup at
# first sign-in). A user who has not will fail with a clear error rather than
# receiving an unusable grant.
resource "seekrit_environment_key_grant" "alice_production" {
  environment_id = seekrit_environment.production.id
  principal_type = "user"
  principal_id   = "usr_5tBnKq8WvZ2"
}
