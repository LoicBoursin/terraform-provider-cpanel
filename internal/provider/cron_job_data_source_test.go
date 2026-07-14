package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccCronJobDataSource(t *testing.T) {
	command := testAccCronCommand("data-source")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCronJobsDestroyed(command),
		Steps: []resource.TestStep{
			{
				Config: testAccCronJobDataSourceConfig(command),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "command", command),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "minute", "13"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "hour", "5"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "day", "21"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "weekday", "4"),
					resource.TestCheckResourceAttr("data.cpanel_cron_job.test", "month", "10"),
					resource.TestCheckResourceAttrSet("data.cpanel_cron_job.test", "linekey"),
					testAccCheckCronJobExists(command),
				),
			},
		},
	})
}

func testAccCronJobDataSourceConfig(command string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_cron_job" "fixture" {
  command = %q
  minute = "13"
  hour = "5"
  day = "21"
  weekday = "4"
  month = "10"
}

data "cpanel_cron_job" "test" {
  command = cpanel_cron_job.fixture.command
  minute = cpanel_cron_job.fixture.minute
  hour = cpanel_cron_job.fixture.hour
  day = cpanel_cron_job.fixture.day
  weekday = cpanel_cron_job.fixture.weekday
  month = cpanel_cron_job.fixture.month
}
`, command)
}
