package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryindex "terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func TestAccDirectoryIndexDataSource(t *testing.T) {
	directory := testAccDirectoryIndexDirectory("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, directory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryIndexesDestroyed(
			directory,
		),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_directory_index" "fixture" {
  directory = %q
  type      = %q
}

data "cpanel_directory_index" "test" {
  directory = cpanel_directory_index.fixture.directory
}
`, directory, cpaneldirectoryindex.IndexTypeDisabled),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_directory_index.test",
						"directory",
						directory,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_directory_index.test",
						"type",
						cpaneldirectoryindex.IndexTypeDisabled,
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_directory_index.test",
						"absolute_directory",
					),
					testAccCheckDirectoryIndexExists(
						cpaneldirectoryindex.Definition{
							Directory: directory,
							Type:      cpaneldirectoryindex.IndexTypeDisabled,
						},
					),
				),
			},
		},
	})
}
