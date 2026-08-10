package provider

import (
	"fmt"
	"regexp"
	"testing"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccFilesystemTextFileResource(t *testing.T) {
	const resourceName = "cpanel_filesystem_text_file.test"

	initialPath := testAccFilesystemTextFilePath("resource")
	replacementPath := testAccFilesystemTextFilePath("replacement")
	initialContent := "line 1: 100% & plus+\nline 2: caf\u00e9\n"
	replacementContent := "replacement\n"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckFilesystemTextFilesDestroyed(
			initialPath,
			replacementPath,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccFilesystemTextFileResourceConfig(
					initialPath,
					initialContent,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						initialPath,
						initialContent,
						true,
						true,
					),
					testAccCheckFilesystemTextFile(
						initialPath,
						initialContent,
						true,
					),
				),
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					initialPath,
					"",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						initialPath,
						"",
						true,
						true,
					),
					testAccCheckFilesystemTextFile(
						initialPath,
						"",
						true,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialPath,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "path",
				ImportStateVerifyIgnore: []string{
					"content_matches_ownership_marker",
					"owned",
				},
				Check: testAccCheckFilesystemTextFile(
					initialPath,
					"",
					true,
				),
			},
			{
				PreConfig: func() {
					testAccWriteFilesystemTextFile(
						t,
						initialPath,
						"external drift\n",
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"owned",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"content_matches_ownership_marker",
						"false",
					),
				),
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					initialPath,
					"",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						initialPath,
						"",
						true,
						true,
					),
					testAccCheckFilesystemTextFile(
						initialPath,
						"",
						true,
					),
				),
			},
			{
				PreConfig: func() {
					testAccWriteFilesystemTextFileStaleMarker(
						t,
						initialPath,
						"stale marker content\n",
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"owned",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"content_matches_ownership_marker",
						"false",
					),
				),
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					initialPath,
					"",
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						initialPath,
						"",
						true,
						true,
					),
					testAccCheckFilesystemTextFile(
						initialPath,
						"",
						true,
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteFilesystemTextFileTarget(t, initialPath)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					initialPath,
					"",
				),
				Check: testAccCheckFilesystemTextFile(
					initialPath,
					"",
					true,
				),
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					replacementPath,
					replacementContent,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFilesDestroyed(initialPath),
					testAccCheckFilesystemTextFileResourceState(
						replacementPath,
						replacementContent,
						true,
						true,
					),
					testAccCheckFilesystemTextFile(
						replacementPath,
						replacementContent,
						true,
					),
				),
			},
		},
	})
}

func TestAccFilesystemTextFileImportPreservesUnownedFile(t *testing.T) {
	const resourceName = "cpanel_filesystem_text_file.test"

	filePath := testAccFilesystemTextFilePath("imported")
	content := "pre-existing\n"

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateUnmarkedFilesystemTextFile(t, filePath, content)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFilesystemTextFilesDestroyed(filePath),
		Steps: []testresource.TestStep{
			{
				Config: testAccFilesystemTextFileResourceConfig(
					filePath,
					content,
				),
				ResourceName:       resourceName,
				ImportStateId:      filePath,
				ImportState:        true,
				ImportStatePersist: true,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						filePath,
						content,
						false,
						false,
					),
					testAccCheckFilesystemTextFile(
						filePath,
						content,
						false,
					),
				),
			},
			{
				PreConfig: func() {
					testAccWriteFilesystemTextFileMarker(
						t,
						filePath,
						content,
					)
				},
				Config: testAccFilesystemTextFileResourceConfig(
					filePath,
					content,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckFilesystemTextFileResourceState(
						filePath,
						content,
						false,
						false,
					),
					testAccCheckFilesystemTextFile(
						filePath,
						content,
						true,
					),
				),
			},
			{
				Config: testAccFilesystemTextFileEmptyConfig(),
				Check: testAccCheckFilesystemTextFile(
					filePath,
					content,
					true,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteFilesystemTextFileArtifacts(t, filePath)
				},
				Config: testAccFilesystemTextFileEmptyConfig(),
			},
		},
	})
}

func TestAccFilesystemTextFileDestroyRefusesDrift(t *testing.T) {
	filePath := testAccFilesystemTextFilePath("destroy-drift")
	configuredContent := "configured\n"
	driftedContent := "externally changed\n"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckFilesystemTextFilesDestroyed(filePath),
		Steps: []testresource.TestStep{
			{
				Config: testAccFilesystemTextFileResourceConfig(
					filePath,
					configuredContent,
				),
				Check: testAccCheckFilesystemTextFile(
					filePath,
					configuredContent,
					true,
				),
			},
			{
				PreConfig: func() {
					testAccWriteFilesystemTextFile(
						t,
						filePath,
						driftedContent,
					)
				},
				Config:      testAccFilesystemTextFileEmptyConfig(),
				ExpectError: regexp.MustCompile("refusing deletion"),
			},
			{
				Config: testAccFilesystemTextFileResourceConfig(
					filePath,
					configuredContent,
				),
				Check: testAccCheckFilesystemTextFile(
					filePath,
					configuredContent,
					true,
				),
			},
		},
	})
}

func testAccFilesystemTextFileResourceConfig(
	filePath string,
	content string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_filesystem_text_file" "test" {
  path    = %q
  content = %q
}
`, filePath, content)
}

func testAccFilesystemTextFileEmptyConfig() string {
	return "terraform {}\n"
}
