package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLDatabaseDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + `data "cpanel_postgresql_database" "database_read" {
					name = "sc1bolo8774_database_read"
					users = ["sc1bolo8774_user_read"]
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_postgresql_database.database_read", "name", "sc1bolo8774_database_read"),
					resource.TestCheckResourceAttr("data.cpanel_postgresql_database.database_read", "users.#", "1"),
					resource.TestCheckResourceAttr("data.cpanel_postgresql_database.database_read", "users.0", "sc1bolo8774_user_read"),
				),
			},
		},
	})
}
