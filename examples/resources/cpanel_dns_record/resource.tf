resource "cpanel_dns_record" "verification" {
  zone = "example.com"
  name = "_terraform"
  type = "TXT"
  ttl  = 300
  data = ["managed-by-terraform"]
}
