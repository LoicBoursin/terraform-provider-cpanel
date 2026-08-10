resource "cpanel_email_account_suspension" "mailbox" {
  email              = "terraform@example.com"
  login_suspended    = false
  incoming_suspended = false
  outgoing_suspended = true
}
