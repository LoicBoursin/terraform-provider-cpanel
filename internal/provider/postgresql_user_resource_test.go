package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLUserResource(t *testing.T) {
	var password = "kgwFvr4Itufg5Im"
	var passwordNew = "KZ8NDJS72JRBDSIZ982NEDNS"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_user" "user_create" {
						name = "sc1bolo8774_user_create"
						password = "` + password + `"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_postgresql_user.user_create", "name", "sc1bolo8774_user_create"),
					resource.TestCheckResourceAttr("cpanel_postgresql_user.user_create", "password", password),
					resource.TestCheckResourceAttrSet("cpanel_postgresql_user.user_create", "last_updated"),
				),
			},
			// ImportState testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_user" "user_import" {
						name = "sc1bolo8774_user_import"
						password = "` + password + `"
					}
				`,
			},
			{
				ResourceName:                         "cpanel_postgresql_user.user_import",
				ImportStateId:                        "sc1bolo8774_user_import",
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"password", "last_updated"},
			},
			// Update Read testing
			{
				Config: providerConfig + `
					resource "cpanel_postgresql_user" "user_update" {
						name = "sc1bolo8774_user_update"
						password = "` + passwordNew + `"
					}
				`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("cpanel_postgresql_user.user_update", "name", "sc1bolo8774_user_update"),
					resource.TestCheckResourceAttr("cpanel_postgresql_user.user_update", "password", passwordNew),
					resource.TestCheckResourceAttrSet("cpanel_postgresql_user.user_update", "last_updated"),
				),
			},
		},
	})
}
