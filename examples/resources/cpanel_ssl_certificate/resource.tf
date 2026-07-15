resource "cpanel_ssl_certificate" "example" {
  friendly_name = "Example public certificate"
  certificate   = file("${path.module}/certificate.pem")
}
