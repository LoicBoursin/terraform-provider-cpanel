resource "cpanel_ssl_csr" "example" {
  key_id                   = "existing-cpanel-ssl-key-id"
  friendly_name            = "Terraform example CSR"
  domains                  = ["example.com", "www.example.com"]
  country_name             = "FR"
  state_or_province_name   = "Ile-de-France"
  locality_name            = "Paris"
  organization_name        = "Example Organization"
  organizational_unit_name = "Platform"
  email_address            = "admin@example.com"
}
