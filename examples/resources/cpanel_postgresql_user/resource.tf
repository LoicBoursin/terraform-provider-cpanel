variable "postgresql_user_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "cpanel_postgresql_user" "user" {
  name              = "sc1john1234_user"
  password          = var.postgresql_user_password
  password_version  = 1
  delete_on_destroy = false
}
