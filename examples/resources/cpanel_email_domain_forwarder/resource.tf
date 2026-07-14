resource "cpanel_email_domain_forwarder" "legacy" {
  domain      = "example.com"
  destination = "archive.example.net"
}
