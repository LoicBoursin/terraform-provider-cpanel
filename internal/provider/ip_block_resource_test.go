package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	testAccIPBlockIPv4  = "198.51.100.254"
	testAccIPBlockCIDR  = "203.0.113.248/30"
	testAccIPBlockRange = "198.51.100.240-198.51.100.242"
	testAccIPBlockIPv6  = "2001:db8:ffff::254"
)

func TestAccIPBlockResource(t *testing.T) {
	const resourceName = "cpanel_ip_block.test"

	for _, address := range []string{
		testAccIPBlockIPv4,
		testAccIPBlockCIDR,
		testAccIPBlockRange,
		testAccIPBlockIPv6,
		"198.51.100.240/31",
		"198.51.100.242",
		"2001:0db8:ffff::254",
	} {
		testAccRegisterArtifact(address)
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckIPBlocksDestroyed(
			testAccIPBlockIPv4,
			testAccIPBlockCIDR,
			testAccIPBlockRange,
			testAccIPBlockIPv6,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccIPBlockResourceConfig(testAccIPBlockIPv4),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"address",
						testAccIPBlockIPv4,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"start_address",
						testAccIPBlockIPv4,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"end_address",
						testAccIPBlockIPv4,
					),
					testAccCheckIPBlockExists(
						testAccIPBlockIPv4,
						testAccIPBlockIPv4,
						testAccIPBlockIPv4,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        testAccIPBlockIPv4,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "address",
			},
			{
				Config: testAccIPBlockResourceConfig(testAccIPBlockCIDR),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"address",
						testAccIPBlockCIDR,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"start_address",
						"203.0.113.248",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"end_address",
						"203.0.113.251",
					),
					testAccCheckIPBlockExists(
						testAccIPBlockCIDR,
						"203.0.113.248",
						"203.0.113.251",
					),
					testAccCheckIPBlocksDestroyed(testAccIPBlockIPv4),
				),
			},
			{
				Config: testAccIPBlockResourceConfig(testAccIPBlockRange),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"address",
						testAccIPBlockRange,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"start_address",
						"198.51.100.240",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"end_address",
						"198.51.100.242",
					),
					testAccCheckIPBlockExists(
						testAccIPBlockRange,
						"198.51.100.240",
						"198.51.100.242",
					),
					testAccCheckIPBlocksDestroyed(testAccIPBlockCIDR),
				),
			},
			{
				Config: testAccIPBlockResourceConfig(testAccIPBlockIPv6),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"address",
						testAccIPBlockIPv6,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"start_address",
						testAccIPBlockIPv6,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"end_address",
						testAccIPBlockIPv6,
					),
					testAccCheckIPBlockExists(
						testAccIPBlockIPv6,
						testAccIPBlockIPv6,
						testAccIPBlockIPv6,
					),
					testAccCheckIPBlocksDestroyed(testAccIPBlockRange),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteIPBlock(t, testAccIPBlockIPv6)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccIPBlockResourceConfig(testAccIPBlockIPv6),
				Check: testAccCheckIPBlockExists(
					testAccIPBlockIPv6,
					testAccIPBlockIPv6,
					testAccIPBlockIPv6,
				),
			},
		},
	})
}

func testAccIPBlockResourceConfig(address string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_ip_block" "test" {
  address = %q
}
`, address)
}
