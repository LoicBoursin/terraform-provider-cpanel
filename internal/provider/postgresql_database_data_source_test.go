package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLDatabaseDataSource(t *testing.T) {
	databaseName := testAccPostgreSQLName("dds")
	userName := testAccPostgreSQLName("ddsu")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckPostgreSQLDatabasesDestroyed(databaseName),
			testAccCheckPostgreSQLUsersDestroyed(userName),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccPostgreSQLDatabaseDataSourceConfig(databaseName, userName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_postgresql_database.test", "name", databaseName),
					resource.TestCheckResourceAttr("data.cpanel_postgresql_database.test", "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(
						"data.cpanel_postgresql_database.test",
						"users.*",
						userName,
					),
					testAccCheckPostgreSQLDatabaseExists(databaseName, userName),
				),
			},
		},
	})
}

func testAccPostgreSQLDatabaseDataSourceConfig(databaseName, userName string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_postgresql_user" "fixture" {
  name             = %q
  password         = "S3!databaseDataSource-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_postgresql_database" "fixture" {
  name              = %q
  users             = [cpanel_postgresql_user.fixture.name]
  delete_on_destroy = true
}

data "cpanel_postgresql_database" "test" {
  name = cpanel_postgresql_database.fixture.name
}
`, userName, databaseName)
}
