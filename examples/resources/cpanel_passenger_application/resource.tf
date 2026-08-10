resource "cpanel_git_repository" "application" {
  name                       = "Example Passenger application"
  repository_root            = "applications/example"
  delete_contents_on_destroy = false
}

resource "cpanel_passenger_application" "application" {
  name            = "example"
  path            = cpanel_git_repository.application.repository_root
  domain          = "example.com"
  base_uri        = "/application"
  deployment_mode = "production"
  enabled         = false

  environment_variables = {
    APP_ENV = "production"
  }
}
