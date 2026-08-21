output "group_id" {
  description = "The group id (`grp_…`). Pass this to an application environment's `groups` to compose it."
  value       = seekrit_group.this.id
}

output "slug" {
  description = "The group slug, echoed for composing names downstream."
  value       = seekrit_group.this.slug
}

output "environment_ids" {
  description = "Environment slug → id, for writing secrets into the group."
  value       = { for slug, env in seekrit_environment.this : slug => env.id }
}
