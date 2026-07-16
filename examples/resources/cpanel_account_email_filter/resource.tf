resource "cpanel_account_email_filter" "important" {
  name    = "Important account messages"
  enabled = true

  rules = [
    {
      part     = "$header_subject:"
      match    = "contains"
      value    = "[IMPORTANT]"
      operator = "none"
    },
  ]

  actions = [
    {
      action      = "deliver"
      destination = "archive@example.com"
    },
    {
      action      = "finish"
      destination = null
    },
  ]
}
