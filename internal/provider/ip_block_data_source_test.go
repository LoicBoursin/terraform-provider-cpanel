package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testAccIPBlockDataSourceIPv4 = "198.51.100.253"

func TestAccIPBlockDataSource(t *testing.T) {
	testAccRegisterArtifact(testAccIPBlockDataSourceIPv4)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckIPBlocksDestroyed(
			testAccIPBlockDataSourceIPv4,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
resource "cpanel_ip_block" "fixture" {
  address = "198.51.100.253"
}

data "cpanel_ip_block" "test" {
  address = cpanel_ip_block.fixture.address
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_ip_block.test",
						"address",
						testAccIPBlockDataSourceIPv4,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_ip_block.test",
						"start_address",
						testAccIPBlockDataSourceIPv4,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_ip_block.test",
						"end_address",
						testAccIPBlockDataSourceIPv4,
					),
					testAccCheckIPBlockExists(
						testAccIPBlockDataSourceIPv4,
						testAccIPBlockDataSourceIPv4,
						testAccIPBlockDataSourceIPv4,
					),
				),
			},
		},
	})
}
