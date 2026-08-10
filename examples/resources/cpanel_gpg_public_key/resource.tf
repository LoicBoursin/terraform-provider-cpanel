resource "cpanel_gpg_public_key" "example" {
  public_key = file("${path.module}/public-key.asc")
}
