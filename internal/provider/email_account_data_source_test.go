package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailAccountDataSource(t *testing.T) {
	address := testAccEmailAddress(t, "datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailAccountsDestroyed(address),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailAccountDataSourceConfig(address),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account.test",
						"email",
						address,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account.test",
						"quota_mib",
						"50",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account.test",
						"disk_used_bytes",
						"0",
					),
					testAccCheckEmailAccountExists(address, 50),
				),
			},
		},
	})
}

func testAccEmailAccountDataSourceConfig(address string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "fixture" {
  email            = %q
  password         = "S3!emailDataSource-2026"
  password_version = 1
  quota_mib        = 50
  delete_on_destroy = true
}

data "cpanel_email_account" "test" {
  email = cpanel_email_account.fixture.email
}
`, address)
}
