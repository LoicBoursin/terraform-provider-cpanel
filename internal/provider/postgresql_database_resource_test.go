package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLDatabaseResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_database" "database_create" {
						name = "sc1bolo8774_database_create"
						users = ["sc1bolo8774_user_read"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_create", "name", "sc1bolo8774_database_create"),
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_create", "users.#", "1"),
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_create", "users.0", "sc1bolo8774_user_read"),
					resource.TestCheckResourceAttrSet("cpanel_postgresql_database.database_create", "last_updated"),
				),
			},
			// ImportState testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_database" "database_import" {
						name = "sc1bolo8774_database_import"
						users = ["sc1bolo8774_user_read"]
					}
				`,
			},
			{
				ResourceName:                         "cpanel_postgresql_database.database_import",
				ImportStateId:                        "sc1bolo8774_database_import",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"last_updated"},
			},
			// Update and Read testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_user" "user_new" {
						name = "sc1bolo8774_user_new"
						password = "KZ8NDJS72JRBDSIZ982NEDNS"
					}

					resource "cpanel_postgresql_database" "database_update" {
						name = "sc1bolo8774_database_update"
						users = ["sc1bolo8774_user_new"]
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_update", "name", "sc1bolo8774_database_update"),
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_update", "users.#", "1"),
					resource.TestCheckResourceAttr("cpanel_postgresql_database.database_update", "users.0", "sc1bolo8774_user_new"),
					resource.TestCheckResourceAttrSet("cpanel_postgresql_database.database_update", "last_updated"),
				),
			},
		},
	})
}
