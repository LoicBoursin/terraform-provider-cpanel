package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailAccountSuspensionDataSource(t *testing.T) {
	const password = "N6!emailSuspensionDataSource-2026"

	address := testAccEmailAddress(t, "suspensiondatasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailAccountsDestroyed(address),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailAccountSuspensionDataSourceConfig(
					address,
					password,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"email",
						address,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"login_suspended",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"incoming_suspended",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"outgoing_suspended",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"outgoing_held",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_account_suspension.test",
						"has_suspended",
						"false",
					),
				),
			},
		},
	})
}

func testAccEmailAccountSuspensionDataSourceConfig(
	address string,
	password string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "fixture" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 25
  delete_on_destroy = true
}

data "cpanel_email_account_suspension" "test" {
  email = cpanel_email_account.fixture.email
}
`, address, password)
}
