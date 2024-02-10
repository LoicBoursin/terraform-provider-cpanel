resource "cpanel_cron_job" "cron" {
  command = "ls -la"
  minute  = "0"
  hour    = "0"
  day     = "1"
  weekday = "*"
  month   = "1"
}