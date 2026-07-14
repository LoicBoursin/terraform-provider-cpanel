package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMySQLUserDataSource(t *testing.T) {
	name := testAccMySQLName("uds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMySQLUsersDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccMySQLUserDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_mysql_user.test", "name", name),
					testAccCheckMySQLUserExists(name),
				),
			},
		},
	})
}

func testAccMySQLUserDataSourceConfig(name string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "fixture" {
  name             = %q
  password         = "S3!mysqlUserDataSource-2026"
  password_version = 1
  delete_on_destroy = true
}

data "cpanel_mysql_user" "test" {
  name = cpanel_mysql_user.fixture.name
}
`, name)
}
