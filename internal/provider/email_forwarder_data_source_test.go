package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailForwarderDataSource(t *testing.T) {
	address := testAccEmailForwarderAddress(t, "datasource")
	destination := testAccEmailForwarderDestination("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailForwardersDestroyed(
			address,
			destination,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailForwarderDataSourceConfig(address, destination),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_forwarder.test",
						"address",
						address,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_forwarder.test",
						"destination",
						destination,
					),
					testAccCheckEmailForwarderExists(address, destination),
				),
			},
		},
	})
}

func testAccEmailForwarderDataSourceConfig(address, destination string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_forwarder" "fixture" {
  address     = %q
  destination = %q
}

data "cpanel_email_forwarder" "test" {
  address     = cpanel_email_forwarder.fixture.address
  destination = cpanel_email_forwarder.fixture.destination
}
`, address, destination)
}
