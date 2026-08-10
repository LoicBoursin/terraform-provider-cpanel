resource "cpanel_email_filter" "important" {
  account = "inbox@example.com"
  name    = "Important messages"
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
