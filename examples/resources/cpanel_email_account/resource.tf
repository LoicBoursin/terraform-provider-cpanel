variable "email_account_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "cpanel_email_account" "mailbox" {
  email             = "terraform@example.com"
  password          = var.email_account_password
  password_version  = 1
  quota_mib         = 1024
  delete_on_destroy = false
}
