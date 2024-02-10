package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCronJobResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `resource "cpanel_cron_job" "cron_read" {
						command = "echo 'create'"
						minute = "0"
						hour = "0"
						day = "1"
						weekday = "*"
						month = "1"
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "command", "echo 'create'"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "minute", "0"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "hour", "0"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "day", "1"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "weekday", "*"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_read", "month", "1"),
					resource.TestCheckResourceAttrSet("cpanel_cron_job.cron_read", "linekey"),
					resource.TestCheckResourceAttrSet("cpanel_cron_job.cron_read", "last_updated"),
				),
			},
			// Update Read testing
			{
				Config: providerConfig + `resource "cpanel_cron_job" "cron_update" {
						command = "ls -lar"
						minute = "1"
						hour = "1"
						day = "2"
						weekday = "3"
						month = "2"
					}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "command", "ls -lar"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "minute", "1"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "hour", "1"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "day", "2"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "weekday", "3"),
					resource.TestCheckResourceAttr("cpanel_cron_job.cron_update", "month", "2"),
					resource.TestCheckResourceAttrSet("cpanel_cron_job.cron_update", "linekey"),
					resource.TestCheckResourceAttrSet("cpanel_cron_job.cron_update", "last_updated"),
				),
			},
		},
	})
}
