resource "cpanel_notification_preferences" "account" {
  preferences = {
    notify_account_authn_link                           = true
    notify_account_authn_link_notification_disabled     = true
    notify_contact_address_change                       = true
    notify_contact_address_change_notification_disabled = true
    notify_disk_limit                                   = true
    notify_password_change                              = true
    notify_password_change_notification_disabled        = true
    notify_ssl_expiry                                   = true
    notify_twofactorauth_change                         = true
    notify_twofactorauth_change_notification_disabled   = true
  }
}
