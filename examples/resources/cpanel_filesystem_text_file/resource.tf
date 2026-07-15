resource "cpanel_filesystem_text_file" "robots" {
  path    = "public_html/robots.txt"
  content = <<-EOT
    User-agent: *
    Disallow:
  EOT
}
