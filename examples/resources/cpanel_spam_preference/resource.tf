resource "cpanel_spam_preference" "required_score" {
  preference = "required_score"
  values     = ["5.5"]
}
