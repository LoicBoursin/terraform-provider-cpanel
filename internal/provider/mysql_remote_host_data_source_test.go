package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testAccMySQLRemoteHostDataSource = "198.51.100.245"

func TestAccMySQLRemoteHostDataSource(t *testing.T) {
	testAccRegisterArtifact(testAccMySQLRemoteHostDataSource)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckMySQLRemoteHostsDestroyed(
			testAccMySQLRemoteHostDataSource,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "cpanel_mysql_remote_host" "fixture" {
  host = "198.51.100.245"
  note = "terraform-provider-cpanel data source"
}

data "cpanel_mysql_remote_host" "test" {
  host = cpanel_mysql_remote_host.fixture.host
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_mysql_remote_host.test",
						"host",
						testAccMySQLRemoteHostDataSource,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_mysql_remote_host.test",
						"note",
						"terraform-provider-cpanel data source",
					),
					testAccCheckMySQLRemoteHostExists(
						testAccMySQLRemoteHostDataSource,
						"terraform-provider-cpanel data source",
					),
				),
			},
		},
	})
}
