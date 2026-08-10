variable "mailing_list_password" {
  type      = string
  sensitive = true
}

resource "cpanel_email_mailing_list" "announcements" {
  address          = "announcements@example.com"
  password         = var.mailing_list_password
  password_version = 1
  advertised       = false
  archive_private  = true
  subscribe_policy = 3
}
