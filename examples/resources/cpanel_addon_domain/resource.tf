resource "cpanel_addon_domain" "site" {
  domain               = "site.example.net"
  internal_subdomain   = "site-example-net"
  document_root        = "public_html/site"
  delete_document_root = false
}
