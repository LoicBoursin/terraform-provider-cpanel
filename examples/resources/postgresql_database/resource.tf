resource "cpanel_postgresql_database" "database" {
  name  = "sc1john1234_database"
  users = ["sc1john1234_user"]
}