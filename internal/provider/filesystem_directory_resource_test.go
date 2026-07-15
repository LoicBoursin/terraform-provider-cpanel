package provider

import (
	"fmt"
	"testing"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFilesystemDirectoryResource(t *testing.T) {
	const resourceName = "cpanel_filesystem_directory.test"

	initialPath := testAccFilesystemDirectoryPath("resource")
	replacementPath := testAccFilesystemDirectoryPath("replacement")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckFilesystemDirectoriesDestroyed(
			initialPath,
			replacementPath,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccFilesystemDirectoryResourceConfig(initialPath),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"path",
						initialPath,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"permissions",
						"0755",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"owned",
						"true",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_path",
					),
					testAccCheckFilesystemDirectory(initialPath, true),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialPath,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "path",
				ImportStateVerifyIgnore: []string{
					"owned",
				},
				Check: testAccCheckFilesystemDirectory(
					initialPath,
					true,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteFilesystemDirectory(t, initialPath)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccFilesystemDirectoryResourceConfig(initialPath),
				Check: testAccCheckFilesystemDirectory(
					initialPath,
					true,
				),
			},
			{
				Config: testAccFilesystemDirectoryResourceConfig(
					replacementPath,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemDirectoriesDestroyed(initialPath),
					testAccCheckFilesystemDirectory(replacementPath, true),
				),
			},
		},
	})
}

func TestAccFilesystemDirectoryImportPreservesUnmarkedDirectory(t *testing.T) {
	const resourceName = "cpanel_filesystem_directory.test"

	directoryPath := testAccFilesystemDirectoryPath("imported")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateUnmarkedFilesystemDirectory(t, directoryPath)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckFilesystemDirectoriesDestroyed(
			directoryPath,
		),
		Steps: []testresource.TestStep{
			{
				Config:             testAccFilesystemDirectoryResourceConfig(directoryPath),
				ResourceName:       resourceName,
				ImportStateId:      directoryPath,
				ImportState:        true,
				ImportStatePersist: true,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"owned",
						"false",
					),
					testAccCheckFilesystemDirectory(
						directoryPath,
						false,
					),
				),
			},
			{
				PreConfig: func() {
					testAccWriteFilesystemDirectoryMarker(t, directoryPath)
				},
				Config: testAccFilesystemDirectoryResourceConfig(directoryPath),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"owned",
						"false",
					),
					testAccCheckFilesystemDirectoryMarker(
						directoryPath,
						true,
					),
				),
			},
			{
				Config: testAccFilesystemDirectoryEmptyConfig(),
				Check: testAccCheckFilesystemDirectoryMarker(
					directoryPath,
					true,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteFilesystemDirectory(t, directoryPath)
				},
				Config: testAccFilesystemDirectoryEmptyConfig(),
			},
		},
	})
}

func testAccFilesystemDirectoryEmptyConfig() string {
	return "terraform {}\n"
}

func testAccFilesystemDirectoryResourceConfig(directoryPath string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_filesystem_directory" "test" {
  path = %q
}
`, directoryPath)
}
