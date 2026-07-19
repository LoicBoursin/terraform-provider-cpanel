resource "cpanel_git_repository" "website" {
  name                       = "Website"
  repository_root            = "repositories/website"
  source_repository_url      = "https://github.com/example/website.git"
  delete_contents_on_destroy = false
}

# Before changing source_repository_url at the same repository_root, set
# delete_contents_on_destroy to true and apply that change separately.
