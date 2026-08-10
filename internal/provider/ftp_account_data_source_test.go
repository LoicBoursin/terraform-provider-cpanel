package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFTPAccountDataSource(t *testing.T) {
	username := testAccFTPUsername(t, "datasource")
	homeDirectory := testAccFTPHomeDirectory("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFTPAccountsDestroyed(username),
		Steps: []resource.TestStep{
			{
				Config: testAccFTPAccountDataSourceConfig(username, homeDirectory),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_ftp_account.test",
						"username",
						username,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_ftp_account.test",
						"home_directory",
						homeDirectory,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_ftp_account.test",
						"quota_mib",
						"50",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_ftp_account.test",
						"disk_used_mib",
						"0",
					),
					testAccCheckFTPAccountExists(username, homeDirectory, 50),
				),
			},
		},
	})
}

func testAccFTPAccountDataSourceConfig(username, homeDirectory string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_ftp_account" "fixture" {
  username              = %q
  password              = "S3!ftpDataSource-2026"
  password_version      = 1
  home_directory        = %q
  quota_mib             = 50
  delete_on_destroy     = true
  delete_home_directory = true
}

data "cpanel_ftp_account" "test" {
  username = cpanel_ftp_account.fixture.username
}
`, username, homeDirectory)
}
