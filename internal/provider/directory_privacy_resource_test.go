package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryprivacy "terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestDirectoryPrivacyResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewDirectoryPrivacyResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"directory", "auth_name"} {
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

	for _, attributeName := range []string{
		"absolute_directory",
		"auth_type",
		"password_file",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed string", attributeName)
		}
	}

	protected, ok := response.Schema.Attributes["protected"].(resourceschema.BoolAttribute)
	if !ok || !protected.Computed {
		t.Fatal("protected must be a computed boolean")
	}
}

func TestAccDirectoryPrivacyResource(t *testing.T) {
	const resourceName = "cpanel_directory_privacy.test"

	initialDirectory := testAccDirectoryPrivacyDirectory("resource")
	replacementDirectory := testAccDirectoryPrivacyDirectory("replacement")
	initialAuthName := "Terraform private initial"
	updatedAuthName := "Terraform private updated"

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, initialDirectory)
			testAccCreateDirectory(t, replacementDirectory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryPrivaciesDestroyed(
			initialDirectory,
			replacementDirectory,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccDirectoryPrivacyResourceConfig(
					initialDirectory,
					initialAuthName,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"directory",
						initialDirectory,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"auth_name",
						initialAuthName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"auth_type",
						"Basic",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"protected",
						"true",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_directory",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"password_file",
					),
					testAccCheckDirectoryPrivacyExists(
						cpaneldirectoryprivacy.Definition{
							Directory: initialDirectory,
							AuthName:  initialAuthName,
						},
					),
				),
			},
			{
				Config: testAccDirectoryPrivacyResourceConfig(
					initialDirectory,
					updatedAuthName,
				),
				Check: testAccCheckDirectoryPrivacyExists(
					cpaneldirectoryprivacy.Definition{
						Directory: initialDirectory,
						AuthName:  updatedAuthName,
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
					testAccSetDirectoryPrivacy(
						t,
						cpaneldirectoryprivacy.Definition{
							Directory: initialDirectory,
							AuthName:  "Terraform external",
						},
						true,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDirectoryPrivacyResourceConfig(
					initialDirectory,
					updatedAuthName,
				),
				Check: testAccCheckDirectoryPrivacyExists(
					cpaneldirectoryprivacy.Definition{
						Directory: initialDirectory,
						AuthName:  updatedAuthName,
					},
				),
			},
			{
				Config: testAccDirectoryPrivacyResourceConfig(
					replacementDirectory,
					updatedAuthName,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDirectoryPrivacyDisabled(initialDirectory),
					testAccCheckDirectoryPrivacyExists(
						cpaneldirectoryprivacy.Definition{
							Directory: replacementDirectory,
							AuthName:  updatedAuthName,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccSetDirectoryPrivacy(
						t,
						cpaneldirectoryprivacy.Definition{
							Directory: replacementDirectory,
							AuthName:  updatedAuthName,
						},
						false,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDirectoryPrivacyResourceConfig(
					replacementDirectory,
					updatedAuthName,
				),
				Check: testAccCheckDirectoryPrivacyExists(
					cpaneldirectoryprivacy.Definition{
						Directory: replacementDirectory,
						AuthName:  updatedAuthName,
					},
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
				Config: testAccDirectoryPrivacyResourceConfig(
					replacementDirectory,
					updatedAuthName,
				),
				Check: testAccCheckDirectoryPrivacyExists(
					cpaneldirectoryprivacy.Definition{
						Directory: replacementDirectory,
						AuthName:  updatedAuthName,
					},
				),
			},
		},
	})
}

func testAccDirectoryPrivacyResourceConfig(
	directory string,
	authName string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_directory_privacy" "test" {
  directory = %q
  auth_name = %q
}
`, directory, authName)
}
