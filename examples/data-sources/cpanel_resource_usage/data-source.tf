data "cpanel_resource_usage" "current" {}

output "resource_usage" {
  value = {
    for metric in data.cpanel_resource_usage.current.metrics :
    metric.id => {
      usage   = metric.usage
      maximum = metric.maximum
    }
  }
}
