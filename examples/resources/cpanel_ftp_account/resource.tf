variable "ftp_account_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "cpanel_ftp_account" "website" {
  username              = "deploy@example.com"
  password              = var.ftp_account_password
  password_version      = 1
  home_directory        = "public_html"
  quota_mib             = 1024
  delete_on_destroy     = false
  delete_home_directory = false
}
