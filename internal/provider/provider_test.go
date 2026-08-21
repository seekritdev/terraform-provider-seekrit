package provider_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/seekritdev/terraform-provider-seekrit/internal/provider"
)

// testAccProtoV6ProviderFactories wires the in-process provider for acceptance
// tests (no separate binary needed).
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"seekrit": providerserver.NewProtocol6WithError(provider.New("test")()),
}

// testAccPreCheck fails fast if the live-API credentials aren't configured.
// Acceptance tests run only when TF_ACC is set (resource.Test enforces this),
// against a real seekrit API — locally that is `wrangler dev` with AUTH_MODE=dev
// plus a bootstrapped org and admin token:
//
//	SEEKRIT_API_URL=http://localhost:8787 SEEKRIT_ORG=org_… SEEKRIT_TOKEN=skt_… \
//	  TF_ACC=1 go test ./internal/provider/...
func testAccPreCheck(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SEEKRIT_API_URL", "SEEKRIT_ORG", "SEEKRIT_TOKEN"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s must be set for acceptance tests", key)
		}
	}
}

// testAccStructuralConfig exercises the whole provider in one dependency graph:
// app + group, an app-owned and a group-owned environment, a composition link, a
// service token, a key grant that hands that token access to the app environment
// (the DEK unwrap+rewrap path), a write-only secret, the ephemeral read of that
// secret, and each data source resolving something the resources just created.
const testAccStructuralConfig = `
resource "seekrit_application" "web" {
  name = "Web"
  slug = "tf-acc-web"
}

resource "seekrit_group" "shared" {
  name = "Shared"
  slug = "tf-acc-shared"
}

resource "seekrit_environment" "prod" {
  application_id = seekrit_application.web.id
  name          = "Production"
  slug          = "production"
}

resource "seekrit_environment" "shared_prod" {
  group_id = seekrit_group.shared.id
  name     = "Production"
  slug     = "production"
}

resource "seekrit_environment_group" "compose" {
  environment_id = seekrit_environment.prod.id
  group_id       = seekrit_group.shared.id
  position       = 0
}

resource "seekrit_service_token" "ci" {
  name           = "tf-acc-ci"
  environment_id = seekrit_environment.prod.id
  role           = "member"
}

resource "seekrit_environment_key_grant" "ci_prod" {
  environment_id = seekrit_environment.prod.id
  principal_type = "service_token"
  principal_id   = seekrit_service_token.ci.id
}

resource "seekrit_secret" "database_url" {
  environment_id   = seekrit_environment.prod.id
  name             = "DATABASE_URL"
  value_wo         = "postgres://acc:test@db.internal:5432/app"
  value_wo_version = 1
}

# The same value read back through the ephemeral resource, then written into a
# second secret. Terraform will not let an ephemeral value be asserted directly
# (that would mean persisting it), so the way to exercise the read path is to
# consume it — and consuming it into a write-only argument is exactly the
# intended use. A decrypt failure surfaces as an apply error. That the decrypted
# bytes are *correct* is pinned offline by the @seekrit/crypto vector tests in
# internal/crypto.
ephemeral "seekrit_secret" "database_url" {
  environment_id = seekrit_environment.prod.id
  name           = seekrit_secret.database_url.name
}

ephemeral "seekrit_secrets" "prod" {
  environment_id = seekrit_environment.prod.id
}

resource "seekrit_secret" "copied" {
  environment_id   = seekrit_environment.prod.id
  name             = "DATABASE_URL_COPY"
  value_wo         = ephemeral.seekrit_secret.database_url.value
  value_wo_version = 1
}

resource "seekrit_secret" "copied_via_map" {
  environment_id   = seekrit_environment.prod.id
  name             = "DATABASE_URL_FROM_MAP"
  value_wo         = ephemeral.seekrit_secrets.prod.values["DATABASE_URL"]
  value_wo_version = 1
}

data "seekrit_application" "by_slug" {
  slug = seekrit_application.web.slug
}

data "seekrit_environment" "by_owner_and_slug" {
  application_id = seekrit_application.web.id
  slug           = seekrit_environment.prod.slug
}

data "seekrit_service_token" "ci" {
  id = seekrit_service_token.ci.id
}

data "seekrit_organization" "current" {}
`

func TestAccStructuralResources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccStructuralConfig,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("seekrit_application.web", "id"),
					resource.TestCheckResourceAttr("seekrit_application.web", "slug", "tf-acc-web"),
					resource.TestCheckResourceAttrSet("seekrit_group.shared", "id"),
					resource.TestCheckResourceAttrSet("seekrit_environment.prod", "id"),
					resource.TestCheckResourceAttrPair(
						"seekrit_environment.prod", "application_id",
						"seekrit_application.web", "id"),
					resource.TestCheckResourceAttrPair(
						"seekrit_environment.shared_prod", "group_id",
						"seekrit_group.shared", "id"),
					resource.TestCheckResourceAttr("seekrit_environment_group.compose", "position", "0"),
					// The minted token secret is populated and looks like a token.
					resource.TestMatchResourceAttr("seekrit_service_token.ci", "token",
						regexp.MustCompile(`^skt_[0-9A-Za-z]+_`)),
					resource.TestCheckResourceAttrSet("seekrit_service_token.ci", "token_hash"),
					resource.TestCheckResourceAttrSet("seekrit_environment_key_grant.ci_prod", "id"),
					// A first write is version 1, and the plaintext is nowhere in state.
					resource.TestCheckResourceAttr("seekrit_secret.database_url", "version", "1"),
					resource.TestCheckNoResourceAttr("seekrit_secret.database_url", "value_wo"),
					// Data sources resolve to the same objects the resources created.
					resource.TestCheckResourceAttrPair(
						"data.seekrit_application.by_slug", "id",
						"seekrit_application.web", "id"),
					resource.TestCheckResourceAttrPair(
						"data.seekrit_environment.by_owner_and_slug", "id",
						"seekrit_environment.prod", "id"),
					resource.TestCheckResourceAttrPair(
						"data.seekrit_service_token.ci", "public_key_jwk",
						"seekrit_service_token.ci", "public_key_jwk"),
					resource.TestCheckResourceAttr(
						"data.seekrit_organization.current", "role", "admin"),
					// Both copies exist, so both ephemeral reads decrypted.
					resource.TestCheckResourceAttr("seekrit_secret.copied", "version", "1"),
					resource.TestCheckResourceAttr("seekrit_secret.copied_via_map", "version", "1"),
				),
			},
			{
				// Bumping the version is what pushes a new value; the server-side
				// counter follows, which is how we know the write happened.
				Config: strings.Replace(testAccStructuralConfig,
					`value_wo         = "postgres://acc:test@db.internal:5432/app"
  value_wo_version = 1`,
					`value_wo         = "postgres://acc:rotated@db.internal:5432/app"
  value_wo_version = 2`, 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("seekrit_secret.database_url", "version", "2"),
				),
			},
			{
				// `<environment_id>:<name>` import. ImportStateVerify is off: the
				// value is unrecoverable by design, so an imported secret cannot
				// match the created one attribute for attribute.
				ResourceName: "seekrit_secret.database_url",
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs, ok := s.RootModule().Resources["seekrit_secret.database_url"]
					if !ok {
						return "", fmt.Errorf("seekrit_secret.database_url not in state")
					}
					return rs.Primary.ID, nil
				},
			},
			{
				// Structural resources round-trip through import cleanly.
				ResourceName:      "seekrit_application.web",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}
