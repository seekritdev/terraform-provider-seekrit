# By id.
data "seekrit_environment" "by_id" {
  id = "env_9QpZ3vLmKd8"
}

# Or by owner plus slug, which is how you address an environment you did not
# create in Terraform — an owner's slugs are stable, its ids are not memorable.
data "seekrit_environment" "production" {
  application_id = data.seekrit_application.web.id
  slug           = "production"
}
