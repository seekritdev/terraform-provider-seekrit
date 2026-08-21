# terraform-provider-seekrit

Manage [seekrit](../../README.md) as infrastructure-as-code: applications,
groups, environments, composition, service tokens, key grants, and secrets.

The provider is an API client, like the CLI and `seekrit mcp` — all cryptography
happens in this process. It authenticates with an **admin service token** and
drives the `/v1/orgs/…` routes.

**Registry:** `seekritdev/seekrit` — published from the public mirror
[`seekritdev/terraform-provider-seekrit`](https://github.com/seekritdev/terraform-provider-seekrit),
which this directory is synced to (see *Releasing*).

## Zero-knowledge, under Terraform's constraints

The repo-wide invariant (`CLAUDE.md`) is that plaintext never reaches the API.
That part is easy here: the provider is a client, and `internal/crypto` is a Go
port of the same primitives `packages/crypto` implements in WebCrypto.

Terraform adds a second constraint that matters just as much: **state**. A secret
value in `terraform.tfstate` is a plaintext copy of a secret sitting outside
seekrit, which defeats the product regardless of what the API saw. So values move
through Terraform only where Terraform promises not to keep them:

| Direction | Mechanism | Guarantee |
| --- | --- | --- |
| Write | `seekrit_secret.value_wo` — a [write-only argument](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments) (TF 1.11+) | Not written to state or the plan file. |
| Read | `ephemeral.seekrit_secret`, `ephemeral.seekrit_secrets` (TF 1.10+) | Held in memory for one operation, then discarded. |

There is deliberately **no `data "seekrit_secret"`**: a data source stores its
result in state.

`internal/provider/schema_test.go` asserts this structurally rather than by
convention — any attribute whose name suggests a secret value must be write-only
or belong to an ephemeral resource, or the test fails. The one allow-listed
exception is `seekrit_service_token.token`, discussed below.

## Provider configuration

```hcl
terraform {
  required_version = ">= 1.11.0"
  required_providers {
    seekrit = {
      source  = "seekritdev/seekrit"
      version = "~> 0.1"
    }
  }
}

provider "seekrit" {
  endpoint = "https://api.seekrit.dev" # or SEEKRIT_API_URL
  org_id   = "org_…"                   # or SEEKRIT_ORG
  token    = var.seekrit_token         # or SEEKRIT_TOKEN (sensitive)
}
```

Mint the credential with `seekrit token create --admin --name terraform`. Admin
is required because the provider creates applications, mints tokens, and grants
keys.

Two things follow from the token embedding its own private key:

- It **is** key material. Leaking it leaks every environment it holds a grant on.
- The provider can only encrypt for environments it can decrypt. Environments it
  created carry a grant automatically; for one created elsewhere, grant the
  Terraform token access first (`seekrit grant --token skt_… --app … --env …`, or
  a `seekrit_environment_key_grant` resource) — otherwise `seekrit_secret` fails
  with a diagnostic saying exactly that.

If the org has **recovery** enabled, note that environments created here are not
recovery-protected at creation (the provider does not hold the org recovery
public key). Run `seekrit recovery sync` to backfill.

## Resources

| Resource | Purpose | Client-side crypto |
| --- | --- | --- |
| `seekrit_application` | An application (environment container) | — |
| `seekrit_group` | A reusable, org-scoped secret bag | — |
| `seekrit_environment` | An environment owned by an app **or** a group | generates a DEK, wraps it to the provider token |
| `seekrit_environment_group` | Compose a group into an app environment | — |
| `seekrit_service_token` | Mint a service token | mints the keypair + token string locally |
| `seekrit_environment_key_grant` | Grant a principal decrypt access | unwraps the DEK, re-wraps to the recipient |
| `seekrit_secret` | A secret value | AES-256-GCM under the env DEK, AAD-bound |

## Data sources

`seekrit_organization`, `seekrit_application`, `seekrit_group`,
`seekrit_environment`, `seekrit_service_token` — for referencing things Terraform
does not own, and for `public_key_jwk` when granting a key to a token minted
elsewhere. None of them expose a secret value.

## Ephemeral resources

`seekrit_secret` (one value) and `seekrit_secrets` (a whole environment as a
map). The read path — see the table above.

## Modules

[`modules/`](./modules) ships `application`, `group`, and `service-token`. See
[`modules/README.md`](./modules/README.md).

## Mutability

`slug` and ownership are immutable — seekrit treats slugs as identity, since
composition and `GET /v1/resolve` match on them, so changing one replaces the
resource (and replacing an environment destroys its secrets). `name` is editable
in place on every resource that has one. A service token's role, environment
binding, and expiry are immutable because the secret string cannot be re-issued:
changing any of them mints a new token.

### A note on `seekrit_service_token.token`

The minted secret is `Computed` + `Sensitive`, so it is stored in Terraform
state. That is unavoidable for a resource that *creates* a credential — the API
returns it exactly once and there is nowhere else to put it. `aws_iam_access_key.secret`
makes the same trade. It is a newly minted credential, not a pre-existing
customer secret. **Use an encrypted state backend.** The token cannot be
recovered on import, so an imported token can be renamed and revoked from
Terraform but never handed to a consumer again.

## Not covered (and why)

- **Agent access policy** (`agent_identities`, `agent_policies`) needs a browser
  **user session**: policy bundles are signed client-side by a key the API cannot
  forge, and publishing is deliberately closed to admin service tokens so an
  agent cannot widen its own policy. A Terraform provider authenticating as a
  service token is the wrong principal by design — see
  `docs/agent-access-governance.md`.
- **Managed keys (KMS), rotation, leases, and third-party sync** are modellable
  and simply not built yet.

## Local development

Build the binary and point Terraform at it with `dev_overrides` in
`~/.terraformrc`:

```hcl
provider_installation {
  dev_overrides {
    "seekritdev/seekrit" = "/absolute/path/to/apps/terraform-provider-seekrit"
  }
  direct {}
}
```

```sh
cd apps/terraform-provider-seekrit
go build -o terraform-provider-seekrit .
```

Then `terraform plan`/`apply` in a config directory — skip `terraform init` when
using dev overrides. For a local API, `SEEKRIT_API_URL=http://localhost:8787`
against `wrangler dev` (see the repo README, and
`knowledge/playbooks/local-dev-e2e.md`).

## Testing

```sh
# Everything that runs offline: the crypto vectors, the schema checks, the
# zero-knowledge assertion, and HCL syntax for the modules and examples.
go test ./...

# Acceptance tests against a live API (creates and destroys real resources).
SEEKRIT_API_URL=http://localhost:8787 \
SEEKRIT_ORG=org_… \
SEEKRIT_TOKEN=skt_… \
TF_ACC=1 go test ./internal/provider/... -run TestAcc
```

### Crypto vectors

`internal/crypto` is a Go port of the `skt_` token scheme, the `wd1.` DEK
wrapping, and the `sc1.` secret encryption from `@seekrit/crypto`. It is verified
byte-for-byte against vectors emitted by the real WebCrypto implementation — the
same discipline `apps/run` and `apps/provisioner` use for their Rust ports:

```sh
# Regenerate after any change to packages/crypto's token/wrap/secret formats.
pnpm exec tsx testdata/gen-vectors.mts > testdata/vectors.json
go test ./internal/crypto/...
```

## Documentation

`docs/` is generated from the schemas and the `examples/` tree by
[tfplugindocs](https://github.com/hashicorp/terraform-plugin-docs), and is what
the Terraform Registry renders. It is committed, and CI fails if it is stale:

```sh
go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@v0.25.0 generate --provider-name seekrit
```

Prose that is not derived from a schema lives in `templates/index.md.tmpl`.
Change a resource description in Go, then regenerate — never edit `docs/` by
hand.

## Releasing

Two moving parts, because the Terraform Registry can only ingest releases from a
**public** repository:

1. **release-please** (in this monorepo) bumps the version on a `feat`/`fix`
   commit under `apps/terraform-provider-seekrit/**` and tags `v{VERSION}` — a
   bare tag with no component prefix, which the Registry requires.
2. **`sync-terraform-provider-repo.yml`** mirrors this directory to
   `seekritdev/terraform-provider-seekrit` and pushes the matching tag there.
   The mirror's own `release.yml` runs **GoReleaser** on that tag, producing the
   Registry layout — per-platform zips, a **GPG-signed** `_SHA256SUMS`, and
   `terraform-registry-manifest.json` — and the Registry ingests it.

One-time setup, out of band:

- A repo secret `TERRAFORM_PROVIDER_DEPLOY_KEY` here: an SSH deploy key with
  write access to the mirror.
- Secrets `GPG_PRIVATE_KEY` and `PASSPHRASE` **on the mirror repository** (that
  is where GoReleaser runs).
- Connect the provider on registry.terraform.io under the `seekritdev` namespace
  and register the matching **GPG public key** there.
