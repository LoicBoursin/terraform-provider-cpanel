package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLUserDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: providerConfig + `data "cpanel_postgresql_user" "user_read" {
					name = "sc1bolo8774_user_read"
					password = "password"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.cpanel_postgresql_user.user_read", "name", "sc1bolo8774_user_read"),
				),
			},
		},
	})
}
