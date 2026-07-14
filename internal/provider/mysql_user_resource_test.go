package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccMySQLUserResource(t *testing.T) {
	const (
		resourceName = "cpanel_mysql_user.test"
		passwordOne  = "G7!mysqlUserOne-2026"
		passwordTwo  = "KZ8!mysqlUserTwo-2026"
	)

	name := testAccMySQLName("u")
	renamedName := testAccRegisterArtifact(name + "r")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckMySQLUsersDestroyed(
			name,
			renamedName,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccMySQLUserResourceConfig(name, passwordOne),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordOne)),
					),
					testAccCheckMySQLUserExists(name),
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
				Config: testAccMySQLUserResourceConfig(name, passwordTwo),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordTwo)),
					),
					testAccCheckMySQLUserExists(name),
				),
			},
			{
				Config: testAccMySQLUserResourceConfig(renamedName, passwordTwo),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", renamedName),
					testAccCheckMySQLUserExists(renamedName),
					testAccCheckMySQLUsersDestroyed(name),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteMySQLUser(t, renamedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMySQLUserResourceConfig(renamedName, passwordTwo),
				Check:  testAccCheckMySQLUserExists(renamedName),
			},
		},
	})
}

func testAccMySQLUserResourceConfig(name, password string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "test" {
  name              = %q
  password          = %q
  password_version  = %d
  delete_on_destroy = true
}
`, name, password, testAccPasswordVersion(password))
}
