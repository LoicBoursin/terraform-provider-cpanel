package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailDomainForwarderDataSource(t *testing.T) {
	domain := testAccMainDomain(t)
	destination := testAccEmailDomainForwarderDestination("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailDomainForwarderDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailDomainForwarderDataSourceConfig(
					domain,
					destination,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_domain_forwarder.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_domain_forwarder.test",
						"destination",
						destination,
					),
					testAccCheckEmailDomainForwarderExists(domain, destination),
				),
			},
		},
	})
}

func testAccEmailDomainForwarderDataSourceConfig(
	domain string,
	destination string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_domain_forwarder" "fixture" {
  domain      = %q
  destination = %q
}

data "cpanel_email_domain_forwarder" "test" {
  domain = cpanel_email_domain_forwarder.fixture.domain
}
`, domain, destination)
}
