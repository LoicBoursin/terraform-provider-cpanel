package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const (
	testAccMySQLRemoteHostPrimary     = "198.51.100.247"
	testAccMySQLRemoteHostReplacement = "198.51.100.246"
)

func TestAccMySQLRemoteHostResource(t *testing.T) {
	const resourceName = "cpanel_mysql_remote_host.test"

	testAccRegisterArtifact(testAccMySQLRemoteHostPrimary)
	testAccRegisterArtifact(testAccMySQLRemoteHostReplacement)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckMySQLRemoteHostsDestroyed(
			testAccMySQLRemoteHostPrimary,
			testAccMySQLRemoteHostReplacement,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccMySQLRemoteHostResourceConfig(
					testAccMySQLRemoteHostPrimary,
					"terraform-provider-cpanel primary",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"host",
						testAccMySQLRemoteHostPrimary,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"note",
						"terraform-provider-cpanel primary",
					),
					testAccCheckMySQLRemoteHostExists(
						testAccMySQLRemoteHostPrimary,
						"terraform-provider-cpanel primary",
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        testAccMySQLRemoteHostPrimary,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "host",
			},
			{
				Config: testAccMySQLRemoteHostResourceConfig(
					testAccMySQLRemoteHostPrimary,
					"terraform-provider-cpanel replacement note",
				),
				Check: testAccCheckMySQLRemoteHostExists(
					testAccMySQLRemoteHostPrimary,
					"terraform-provider-cpanel replacement note",
				),
			},
			{
				Config: testAccMySQLRemoteHostResourceConfig(
					testAccMySQLRemoteHostReplacement,
					"terraform-provider-cpanel replacement host",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckMySQLRemoteHostExists(
						testAccMySQLRemoteHostReplacement,
						"terraform-provider-cpanel replacement host",
					),
					testAccCheckMySQLRemoteHostsDestroyed(
						testAccMySQLRemoteHostPrimary,
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteMySQLRemoteHost(
						t,
						testAccMySQLRemoteHostReplacement,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMySQLRemoteHostResourceConfig(
					testAccMySQLRemoteHostReplacement,
					"terraform-provider-cpanel replacement host",
				),
				Check: testAccCheckMySQLRemoteHostExists(
					testAccMySQLRemoteHostReplacement,
					"terraform-provider-cpanel replacement host",
				),
			},
		},
	})
}

func testAccMySQLRemoteHostResourceConfig(host, note string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_remote_host" "test" {
  host = %q
  note = %q
}
`, host, note)
}
