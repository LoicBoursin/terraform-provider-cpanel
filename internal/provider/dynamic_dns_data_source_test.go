package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDynamicDNSDataSource(t *testing.T) {
	domain := testAccDynamicDNSDomain(t, "datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDynamicDNSDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_dynamic_dns" "fixture" {
  domain      = %q
  description = "Data source endpoint"
}

data "cpanel_dynamic_dns" "test" {
  domain = cpanel_dynamic_dns.fixture.domain
}
`, domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_dynamic_dns.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dynamic_dns.test",
						"description",
						"Data source endpoint",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_dynamic_dns.test",
						"webcall_id",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_dynamic_dns.test",
						"webcall_url",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_dynamic_dns.test",
						"created_at",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dynamic_dns.test",
						"ipv4.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dynamic_dns.test",
						"ipv6.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dynamic_dns.test",
						"last_run_times.#",
						"0",
					),
					testAccCheckDynamicDNSExists(
						domain,
						"Data source endpoint",
					),
				),
			},
		},
	})
}
