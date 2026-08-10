package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFilesystemTextFileDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_filesystem_text_file.test"

	filePath := testAccFilesystemTextFilePath("datasource")
	content := "data source: 100% & plus+\n"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFilesystemTextFilesDestroyed(filePath),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_filesystem_text_file" "fixture" {
  path    = %q
  content = %q
}

data "cpanel_filesystem_text_file" "test" {
  path = cpanel_filesystem_text_file.fixture.path
}
`, filePath, content),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"path",
						filePath,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"content",
						content,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"permissions",
						"0644",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"size_bytes",
						fmt.Sprintf("%d", len([]byte(content))),
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"content_sha256",
						filesystemTextFileContentSHA256(content),
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"absolute_path",
					),
					testAccCheckFilesystemTextFile(
						filePath,
						content,
						true,
					),
				),
			},
		},
	})
}
