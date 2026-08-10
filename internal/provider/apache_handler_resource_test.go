package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelapachehandler "terraform-provider-cpanel/internal/cpanel/apachehandler"
)

func TestApacheHandlerResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewApacheHandlerResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"extension", "handler"} {
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
}

func TestAccApacheHandlerResource(t *testing.T) {
	const resourceName = "cpanel_apache_handler.test"

	initialExtension := testAccApacheHandlerExtension("resource")
	replacementExtension := testAccApacheHandlerExtension("replacement")
	initialHandler := testAccApacheHandlerName("initial")
	updatedHandler := testAccApacheHandlerName("updated")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckApacheHandlersDestroyed(
			initialExtension,
			replacementExtension,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccApacheHandlerResourceConfig(
					initialExtension,
					initialHandler,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"extension",
						initialExtension,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"handler",
						initialHandler,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"origin",
						"user",
					),
					testAccCheckApacheHandlerExists(
						cpanelapachehandler.Definition{
							Extension: initialExtension,
							Handler:   initialHandler,
						},
					),
				),
			},
			{
				Config: testAccApacheHandlerResourceConfig(
					initialExtension,
					updatedHandler,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"handler",
						updatedHandler,
					),
					testAccCheckApacheHandlerExists(
						cpanelapachehandler.Definition{
							Extension: initialExtension,
							Handler:   updatedHandler,
						},
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialExtension,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "extension",
			},
			{
				PreConfig: func() {
					testAccReplaceApacheHandler(
						t,
						cpanelapachehandler.Definition{
							Extension: initialExtension,
							Handler: testAccApacheHandlerName(
								"external",
							),
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccApacheHandlerResourceConfig(
					initialExtension,
					updatedHandler,
				),
				Check: testAccCheckApacheHandlerExists(
					cpanelapachehandler.Definition{
						Extension: initialExtension,
						Handler:   updatedHandler,
					},
				),
			},
			{
				Config: testAccApacheHandlerResourceConfig(
					replacementExtension,
					updatedHandler,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckApacheHandlerExists(
						cpanelapachehandler.Definition{
							Extension: replacementExtension,
							Handler:   updatedHandler,
						},
					),
					testAccCheckApacheHandlersDestroyed(initialExtension),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteApacheHandler(t, replacementExtension)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccApacheHandlerResourceConfig(
					replacementExtension,
					updatedHandler,
				),
				Check: testAccCheckApacheHandlerExists(
					cpanelapachehandler.Definition{
						Extension: replacementExtension,
						Handler:   updatedHandler,
					},
				),
			},
		},
	})
}

func testAccApacheHandlerResourceConfig(
	extension string,
	handler string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_apache_handler" "test" {
  extension = %q
  handler   = %q
}
`, extension, handler)
}
