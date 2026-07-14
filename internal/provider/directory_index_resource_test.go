package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryindex "terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func TestDirectoryIndexResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewDirectoryIndexResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"directory", "type"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok {
			t.Fatalf(
				"%s has type %T, want schema.StringAttribute",
				attributeName,
				response.Schema.Attributes[attributeName],
			)
		}
		if !attribute.Required {
			t.Fatalf("%s must be required", attributeName)
		}
	}

	absoluteDirectory, ok := response.Schema.Attributes["absolute_directory"].(resourceschema.StringAttribute)
	if !ok || !absoluteDirectory.Computed {
		t.Fatal("absolute_directory must be a computed string")
	}
}

func TestAccDirectoryIndexResource(t *testing.T) {
	const resourceName = "cpanel_directory_index.test"

	initialDirectory := testAccDirectoryIndexDirectory("resource")
	replacementDirectory := testAccDirectoryIndexDirectory("replacement")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, initialDirectory)
			testAccCreateDirectory(t, replacementDirectory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryIndexesDestroyed(
			initialDirectory,
			replacementDirectory,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccDirectoryIndexResourceConfig(
					initialDirectory,
					cpaneldirectoryindex.IndexTypeDisabled,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"directory",
						initialDirectory,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						cpaneldirectoryindex.IndexTypeDisabled,
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_directory",
					),
					testAccCheckDirectoryIndexExists(
						cpaneldirectoryindex.Definition{
							Directory: initialDirectory,
							Type:      cpaneldirectoryindex.IndexTypeDisabled,
						},
					),
				),
			},
			{
				Config: testAccDirectoryIndexResourceConfig(
					initialDirectory,
					cpaneldirectoryindex.IndexTypeFancy,
				),
				Check: testAccCheckDirectoryIndexExists(
					cpaneldirectoryindex.Definition{
						Directory: initialDirectory,
						Type:      cpaneldirectoryindex.IndexTypeFancy,
					},
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialDirectory,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "directory",
			},
			{
				PreConfig: func() {
					testAccSetDirectoryIndex(
						t,
						cpaneldirectoryindex.Definition{
							Directory: initialDirectory,
							Type:      cpaneldirectoryindex.IndexTypeStandard,
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDirectoryIndexResourceConfig(
					initialDirectory,
					cpaneldirectoryindex.IndexTypeFancy,
				),
				Check: testAccCheckDirectoryIndexExists(
					cpaneldirectoryindex.Definition{
						Directory: initialDirectory,
						Type:      cpaneldirectoryindex.IndexTypeFancy,
					},
				),
			},
			{
				Config: testAccDirectoryIndexResourceConfig(
					replacementDirectory,
					cpaneldirectoryindex.IndexTypeFancy,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDirectoryIndexExists(
						cpaneldirectoryindex.Definition{
							Directory: initialDirectory,
							Type:      cpaneldirectoryindex.IndexTypeInherit,
						},
					),
					testAccCheckDirectoryIndexExists(
						cpaneldirectoryindex.Definition{
							Directory: replacementDirectory,
							Type:      cpaneldirectoryindex.IndexTypeFancy,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteDirectory(t, replacementDirectory)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				PreConfig: func() {
					testAccCreateDirectory(t, replacementDirectory)
				},
				Config: testAccDirectoryIndexResourceConfig(
					replacementDirectory,
					cpaneldirectoryindex.IndexTypeFancy,
				),
				Check: testAccCheckDirectoryIndexExists(
					cpaneldirectoryindex.Definition{
						Directory: replacementDirectory,
						Type:      cpaneldirectoryindex.IndexTypeFancy,
					},
				),
			},
		},
	})
}

func testAccDirectoryIndexResourceConfig(
	directory string,
	indexType string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_directory_index" "test" {
  directory = %q
  type      = %q
}
`, directory, indexType)
}
