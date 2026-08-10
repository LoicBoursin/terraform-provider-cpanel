resource "cpanel_mime_type" "manifest" {
  type       = "application/manifest+json"
  extensions = [".webmanifest"]
}
