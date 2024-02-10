package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCronJobDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + `data "cpanel_cron_job" "cron_read" {
					command = "ls -la"
					minute = "0"
					hour = "0"	
					day = "1"	
					weekday = "*"
					month = "1"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "command", "ls -la"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "minute", "0"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "hour", "0"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "day", "1"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "weekday", "*"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.cron_read", "month", "1"),
					resource.TestCheckResourceAttrSet("data.cpanel_cron_job.cron_read", "linekey"),
					resource.TestCheckResourceAttrSet("data.cpanel_cron_job.cron_read", "last_updated"),
				),
			},
		},
	})
}
