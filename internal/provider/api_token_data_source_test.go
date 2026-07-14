package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAPITokenDataSource(t *testing.T) {
	name := testAccAPITokenName("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAPITokensDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_api_token" "fixture" {
  name = %q
}

data "cpanel_api_token" "test" {
  name = cpanel_api_token.fixture.name
}
`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_api_token.test",
						"name",
						name,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_api_token.test",
						"expires_at",
						"0",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_api_token.test",
						"created_at",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_api_token.test",
						"has_full_access",
						"true",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_api_token.test",
						"features.#",
						"0",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_api_token.test",
						"whitelist_ips.#",
						"0",
					),
					testAccCheckAPITokenExists(name, 0),
				),
			},
		},
	})
}
