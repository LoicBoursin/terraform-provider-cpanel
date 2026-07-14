data "cpanel_dns_record" "verification" {
  zone       = "example.com"
  line_index = 42
}
