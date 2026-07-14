variable "mysql_user_password" {
  type      = string
  sensitive = true
  ephemeral = true
}

resource "cpanel_mysql_user" "user" {
  name              = "sc1john1234_user"
  password          = var.mysql_user_password
  password_version  = 1
  delete_on_destroy = false
}
