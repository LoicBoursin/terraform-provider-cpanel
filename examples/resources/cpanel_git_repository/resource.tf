resource "cpanel_git_repository" "website" {
  name                       = "Website"
  repository_root            = "repositories/website"
  source_repository_url      = "https://github.com/example/website.git"
  delete_contents_on_destroy = false
}
