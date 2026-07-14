package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMySQLDatabaseDataSource(t *testing.T) {
	databaseName := testAccMySQLName("dds")
	userName := testAccMySQLName("ddsu")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckMySQLDatabasesDestroyed(databaseName),
			testAccCheckMySQLUsersDestroyed(userName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccMySQLDatabaseDataSourceConfig(databaseName, userName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_mysql_database.test", "name", databaseName),
					resource.TestCheckResourceAttr("data.cpanel_mysql_database.test", "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(
						"data.cpanel_mysql_database.test",
						"users.*",
						userName,
					),
					testAccCheckMySQLDatabaseExists(databaseName, userName),
				),
			},
		},
	})
}

func testAccMySQLDatabaseDataSourceConfig(databaseName, userName string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "fixture" {
  name             = %q
  password         = "S3!mysqlDatabaseDataSource-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_mysql_database" "fixture" {
  name              = %q
  users             = [cpanel_mysql_user.fixture.name]
  delete_on_destroy = true
}

data "cpanel_mysql_database" "test" {
  name = cpanel_mysql_database.fixture.name
}
`, userName, databaseName)
}
