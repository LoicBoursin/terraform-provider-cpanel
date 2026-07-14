package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryprivacy "terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestAccDirectoryPrivacyUserDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_directory_privacy_user.test"

	directory := testAccDirectoryPrivacyDirectory("userdatasource")
	username := testAccDirectoryPrivacyUsername("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, directory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryPrivacyUsersDestroyed(
			cpaneldirectoryprivacy.UserDefinition{
				Directory: directory,
				Username:  username,
			},
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_directory_privacy" "fixture" {
  directory = %q
  auth_name = "Terraform authorized users"
}

resource "cpanel_directory_privacy_user" "fixture" {
  directory        = cpanel_directory_privacy.fixture.directory
  username         = %q
  password         = "Directory-User-Data-2026!"
  password_version = 1
  delete_on_destroy = true
}

data "cpanel_directory_privacy_user" "test" {
  directory = cpanel_directory_privacy_user.fixture.directory
  username  = cpanel_directory_privacy_user.fixture.username
}
`, directory, username),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"directory",
						directory,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"username",
						username,
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"absolute_directory",
					),
					testAccCheckDirectoryPrivacyUserExists(
						cpaneldirectoryprivacy.UserDefinition{
							Directory: directory,
							Username:  username,
						},
					),
				),
			},
		},
	})
}
