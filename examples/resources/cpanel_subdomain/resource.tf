resource "cpanel_subdomain" "app" {
  domain               = "app.example.com"
  document_root        = "public_html/app"
  delete_document_root = false
}
