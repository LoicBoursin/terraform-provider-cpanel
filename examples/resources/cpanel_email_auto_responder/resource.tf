resource "cpanel_email_auto_responder" "away" {
  email          = "away@example.com"
  from           = "Example Team"
  subject        = "We received your message"
  body           = "We will reply during the next business day."
  charset        = "UTF-8"
  interval_hours = 8
  is_html        = false
  start_unix     = 0
  stop_unix      = 0
}
