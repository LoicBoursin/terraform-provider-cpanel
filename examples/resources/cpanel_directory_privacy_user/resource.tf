resource "cpanel_directory_privacy" "downloads" {
  directory = "public_html/downloads"
  auth_name = "Restricted downloads"
}

variable "directory_privacy_user_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "cpanel_directory_privacy_user" "release_bot" {
  directory         = cpanel_directory_privacy.downloads.directory
  username          = "release-bot"
  password          = var.directory_privacy_user_password
  password_version  = 1
  delete_on_destroy = false
}
