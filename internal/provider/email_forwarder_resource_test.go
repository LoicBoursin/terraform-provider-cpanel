package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailForwarderResource(t *testing.T) {
	const resourceName = "cpanel_email_forwarder.test"

	address := testAccEmailForwarderAddress(t, "resource")
	firstDestination := testAccEmailForwarderDestination("first")
	secondDestination := testAccEmailForwarderDestination("second")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailForwardersDestroyed(
			address,
			firstDestination,
			secondDestination,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailForwarderResourceConfig(address, firstDestination),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "address", address),
					resource.TestCheckResourceAttr(
						resourceName,
						"destination",
						firstDestination,
					),
					testAccCheckEmailForwarderExists(address, firstDestination),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        address + "|" + firstDestination,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "address",
			},
			{
				Config: testAccEmailForwarderResourceConfig(address, secondDestination),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"destination",
						secondDestination,
					),
					testAccCheckEmailForwarderExists(address, secondDestination),
					testAccCheckEmailForwardersDestroyed(address, firstDestination),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailForwarder(t, address, secondDestination)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailForwarderResourceConfig(address, secondDestination),
				Check:  testAccCheckEmailForwarderExists(address, secondDestination),
			},
		},
	})
}

func testAccEmailForwarderResourceConfig(address, destination string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_forwarder" "test" {
  address     = %q
  destination = %q
}
`, address, destination)
}
