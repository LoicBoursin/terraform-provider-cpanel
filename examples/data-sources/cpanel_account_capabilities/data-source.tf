data "cpanel_account_capabilities" "current" {}

locals {
  passenger_available = data.cpanel_account_capabilities.current.features["passengerapps"]
}
