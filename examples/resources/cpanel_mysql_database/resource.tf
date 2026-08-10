resource "cpanel_mysql_database" "database" {
  name              = "sc1john1234_database"
  users             = ["sc1john1234_user"]
  delete_on_destroy = false
}
