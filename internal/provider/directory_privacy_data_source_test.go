package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryprivacy "terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestAccDirectoryPrivacyDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_directory_privacy.test"

	directory := testAccDirectoryPrivacyDirectory("datasource")
	authName := "Terraform private data source"

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, directory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryPrivaciesDestroyed(
			directory,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_directory_privacy" "fixture" {
  directory = %q
  auth_name = %q
}

data "cpanel_directory_privacy" "test" {
  directory = cpanel_directory_privacy.fixture.directory
}
`, directory, authName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"directory",
						directory,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"auth_name",
						authName,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"auth_type",
						"Basic",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"protected",
						"true",
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"absolute_directory",
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"password_file",
					),
					testAccCheckDirectoryPrivacyExists(
						cpaneldirectoryprivacy.Definition{
							Directory: directory,
							AuthName:  authName,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccSetDirectoryPrivacy(
						t,
						cpaneldirectoryprivacy.Definition{
							Directory: directory,
							AuthName:  authName,
						},
						false,
					)
				},
				Config: providerConfig + fmt.Sprintf(`
data "cpanel_directory_privacy" "test" {
  directory = %q
}
`, directory),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"directory",
						directory,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"auth_name",
						"",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"auth_type",
						"None",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"password_file",
						"",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"protected",
						"false",
					),
					testAccCheckDirectoryPrivacyDisabled(directory),
				),
			},
		},
	})
}
