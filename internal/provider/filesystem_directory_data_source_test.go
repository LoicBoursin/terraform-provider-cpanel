package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFilesystemDirectoryDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_filesystem_directory.test"

	directoryPath := testAccFilesystemDirectoryPath("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckFilesystemDirectoriesDestroyed(
			directoryPath,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_filesystem_directory" "fixture" {
  path = %q
}

data "cpanel_filesystem_directory" "test" {
  path = cpanel_filesystem_directory.fixture.path
}
`, directoryPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"path",
						directoryPath,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"permissions",
						"0755",
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"absolute_path",
					),
					testAccCheckFilesystemDirectory(
						directoryPath,
						true,
					),
				),
			},
		},
	})
}
