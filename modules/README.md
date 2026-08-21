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

These modules are not published to the Terraform Registry (which wants one
repository per module). Source them from the provider's repository with a pinned
tag:

```hcl
module "web" {
  source = "git::https://github.com/seekritdev/terraform-provider-seekrit.git//modules/application?ref=v0.1.0"

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

`ref` is not optional in practice. Without it `terraform init` tracks the default
branch, which means a fresh init can change what your infrastructure is.

Each module declares `required_providers` but never a `provider` block —
credentials come from the root module. That is the standard split, and it is what
lets one set of provider credentials drive many modules.

## Slugs are identity

Every module treats `slug` as immutable, because seekrit does: slugs are what
group composition matches on and what `GET /v1/resolve` addresses. Changing a
slug replaces the resource, and replacing an environment destroys the secrets in
it. Rename the `name` freely; treat the `slug` as permanent.
