package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccPostgreSQLUserResource(t *testing.T) {
	const (
		resourceName = "cpanel_postgresql_user.test"
		passwordOne  = "G7!vr4Itufg5Im-2026"
		passwordTwo  = "KZ8!DJS72JRBDSIZ-2026"
	)

	name := testAccPostgreSQLName("u")
	renamedName := testAccRegisterArtifact(name + "r")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckPostgreSQLUsersDestroyed(
			name,
			renamedName,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccPostgreSQLUserResourceConfig(name, passwordOne),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordOne)),
					),
					testAccCheckPostgreSQLUserExists(name),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        name,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore: []string{
					"password",
					"password_version",
					"delete_on_destroy",
				},
			},
			{
				Config: testAccPostgreSQLUserResourceConfig(name, passwordTwo),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordTwo)),
					),
					testAccCheckPostgreSQLUserExists(name),
				),
			},
			{
				Config: testAccPostgreSQLUserResourceConfig(renamedName, passwordTwo),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", renamedName),
					testAccCheckPostgreSQLUserExists(renamedName),
					testAccCheckPostgreSQLUsersDestroyed(name),
				),
			},
			{
				PreConfig: func() {
					testAccDeletePostgreSQLUser(t, renamedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccPostgreSQLUserResourceConfig(renamedName, passwordTwo),
				Check:  testAccCheckPostgreSQLUserExists(renamedName),
			},
		},
	})
}

func testAccPostgreSQLUserResourceConfig(name, password string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_postgresql_user" "test" {
  name              = %q
  password          = %q
  password_version  = %d
  delete_on_destroy = true
}
`, name, password, testAccPasswordVersion(password))
}
