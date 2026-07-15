resource "cpanel_log_settings" "account" {
  archive_logs   = true
  prune_archives = true
  retention_days = -1
}
