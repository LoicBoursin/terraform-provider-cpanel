package provider

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelmimetype "terraform-provider-cpanel/internal/cpanel/mimetype"
)

func TestMIMETypeResourceSchemaUsesExtensionSet(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewMIMETypeResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	attribute, ok := response.Schema.Attributes["extensions"].(resourceschema.SetAttribute)
	if !ok {
		t.Fatalf(
			"extensions has type %T, want schema.SetAttribute",
			response.Schema.Attributes["extensions"],
		)
	}
	if !attribute.Required {
		t.Fatal("extensions must be required")
	}
}

func TestAccMIMETypeResource(t *testing.T) {
	const resourceName = "cpanel_mime_type.test"

	initialType := testAccMIMEType("resource")
	replacementType := testAccMIMEType("replacement")
	firstExtension := testAccMIMEExtension("first")
	secondExtension := testAccMIMEExtension("second")
	thirdExtension := testAccMIMEExtension("third")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckMIMETypesDestroyed(
			initialType,
			replacementType,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccMIMETypeResourceConfig(
					initialType,
					[]string{firstExtension, secondExtension},
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						initialType,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"extensions.#",
						"2",
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"extensions.*",
						firstExtension,
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"extensions.*",
						secondExtension,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"origin",
						"user",
					),
					testAccCheckMIMETypeExists(cpanelmimetype.Definition{
						Type: initialType,
						Extensions: []string{
							firstExtension,
							secondExtension,
						},
					}),
				),
			},
			{
				Config: testAccMIMETypeResourceConfig(
					initialType,
					[]string{secondExtension, thirdExtension},
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"extensions.#",
						"2",
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"extensions.*",
						secondExtension,
					),
					testresource.TestCheckTypeSetElemAttr(
						resourceName,
						"extensions.*",
						thirdExtension,
					),
					testAccCheckMIMETypeExists(cpanelmimetype.Definition{
						Type: initialType,
						Extensions: []string{
							secondExtension,
							thirdExtension,
						},
					}),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialType,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "type",
			},
			{
				PreConfig: func() {
					testAccReplaceMIMEType(
						t,
						cpanelmimetype.Definition{
							Type: initialType,
							Extensions: []string{
								testAccMIMEExtension("external"),
							},
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMIMETypeResourceConfig(
					initialType,
					[]string{secondExtension, thirdExtension},
				),
				Check: testAccCheckMIMETypeExists(
					cpanelmimetype.Definition{
						Type: initialType,
						Extensions: []string{
							secondExtension,
							thirdExtension,
						},
					},
				),
			},
			{
				Config: testAccMIMETypeResourceConfig(
					replacementType,
					[]string{secondExtension, thirdExtension},
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckMIMETypeExists(cpanelmimetype.Definition{
						Type: replacementType,
						Extensions: []string{
							secondExtension,
							thirdExtension,
						},
					}),
					testAccCheckMIMETypesDestroyed(initialType),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteMIMEType(t, replacementType)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMIMETypeResourceConfig(
					replacementType,
					[]string{secondExtension, thirdExtension},
				),
				Check: testAccCheckMIMETypeExists(
					cpanelmimetype.Definition{
						Type: replacementType,
						Extensions: []string{
							secondExtension,
							thirdExtension,
						},
					},
				),
			},
		},
	})
}

func testAccMIMETypeResourceConfig(
	mimeTypeName string,
	extensions []string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mime_type" "test" {
  type       = %q
  extensions = [%s]
}
`, mimeTypeName, terraformQuotedStringList(extensions))
}

func terraformQuotedStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}

	return strings.Join(quoted, ", ")
}
