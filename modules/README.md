# Modules

Three Terraform modules for the shapes that otherwise repeat in every seekrit
configuration. They are thin on purpose: each one encapsulates a *pairing* the
provider cannot enforce on its own, and nothing else.

| Module | Encapsulates | Why it exists |
| --- | --- | --- |
| [`application`](./application) | app + environments + group composition + per-environment runtime tokens and their grants | Four resources per environment, written the same way every time. |
| [`group`](./group) | a group + its environments (+ grants) | Group environments are matched to application environments **by slug**; the module makes that lineup one input. |
| [`service-token`](./service-token) | one token + grants on N environments | A token and its grants are independently valid and jointly required. Splitting them is the most common way to ship a credential that authenticates and then resolves nothing. |

Secret **values** are deliberately not module inputs. A value has to arrive
through an ephemeral variable or a write-only argument, and threading it through
a module boundary adds a hop without removing any of the constraints — so
`seekrit_secret` resources belong in the configuration that owns the values. See
[`examples/complete`](../examples/complete) for the pattern.

## Using them

These modules are not published to the Terraform Registry. Source them from the
provider's repository with a pinned tag:

<!-- x-release-please-start-version -->

```hcl
module "web" {
  source = "git::https://github.com/seekritdev/terraform-provider-seekrit.git//modules/application?ref=v1.0.0"

  name = "Web"
  slug = "web"

  environments = {
    production = { groups = [module.shared.group_id] }
    staging    = {}
  }

  runtime_tokens = {
    ci = { environment = "production" }
  }
}
```

<!-- x-release-please-end -->

`ref` is not optional in practice. Without it `terraform init` tracks the default
branch, which means a fresh init can change what your infrastructure is.

## Why they are not on the Registry

Not because the Registry forbids several modules in one repository — it does not.
The blocker is the repository **name**: a published module's repo must be called
`terraform-<PROVIDER>-<NAME>`, and this one is `terraform-provider-seekrit`,
which is the provider's required name. A listing would mean a second repository.

Inside such a repository, a `modules/` subdirectory *is* parsed: the Registry
publishes the repo root as the module and lists everything under `modules/` as
addressable submodules, exactly as `terraform-aws-modules/ecs/aws//modules/service`
does. So one new repo — say `terraform-seekrit-secrets`, addressed
`seekritdev/secrets/seekrit` — could carry all three of these, with a
whole-organization module at the root and these three demoted to its `modules/`.

What keeps that on the shelf is **version skew**, not effort. Living here, these
modules share the provider's tag, so the modules at any given tag are by
construction the ones written against that provider release. Split out, they get
their own version
numbers and you own a compatibility matrix between them and the provider, plus a
release train per repository. A Registry listing buys discoverability and a
rendered docs page; it does not buy correctness. Revisit when someone outside
this repository is actually looking for these.

Each module declares `required_providers` but never a `provider` block —
credentials come from the root module. That is the standard split, and it is what
lets one set of provider credentials drive many modules.

## Slugs are identity

Every module treats `slug` as immutable, because seekrit does: slugs are what
group composition matches on and what `GET /v1/resolve` addresses. Changing a
slug replaces the resource, and replacing an environment destroys the secrets in
it. Rename the `name` freely; treat the `slug` as permanent.
