resource "cpanel_directory_privacy" "downloads" {
  directory = "public_html/downloads"
  auth_name = "Restricted downloads"
}
