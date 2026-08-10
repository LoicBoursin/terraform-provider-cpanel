resource "cpanel_email_routing" "mail" {
  domain = "mail.example.com"
  mode   = "remote"
}
