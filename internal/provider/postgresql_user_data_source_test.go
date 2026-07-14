package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLUserDataSource(t *testing.T) {
	name := testAccPostgreSQLName("uds")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckPostgreSQLUsersDestroyed(name),
		Steps: []resource.TestStep{
			{
				Config: testAccPostgreSQLUserDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_postgresql_user.test", "name", name),
					testAccCheckPostgreSQLUserExists(name),
				),
			},
		},
	})
}

func testAccPostgreSQLUserDataSourceConfig(name string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_postgresql_user" "fixture" {
  name             = %q
  password         = "N7!dataSourceFixture-2026"
  password_version = 1
  delete_on_destroy = true
}

data "cpanel_postgresql_user" "test" {
  name = cpanel_postgresql_user.fixture.name
}
`, name)
}
