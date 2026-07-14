resource "cpanel_redirect" "legacy" {
  domain      = "example.com"
  source      = "/old"
  destination = "https://example.com/new"
  type        = "permanent"
  www_mode    = "both"
  wildcard    = false
}
