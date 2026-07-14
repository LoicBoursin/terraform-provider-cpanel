package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpaneldirectoryprivacy "terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

func TestDirectoryPrivacyUserResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewDirectoryPrivacyUserResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{
		"directory",
		"username",
	} {
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

	password, ok := response.Schema.Attributes["password"].(resourceschema.StringAttribute)
	if !ok || !password.Optional || !password.Sensitive || !password.WriteOnly {
		t.Fatal("password must be optional, sensitive, and write-only")
	}
	passwordVersion, ok := response.Schema.Attributes["password_version"].(resourceschema.Int64Attribute)
	if !ok || !passwordVersion.Optional {
		t.Fatal("password_version must be an optional integer")
	}
	deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
	if !ok || !deleteOnDestroy.Optional || !deleteOnDestroy.Computed {
		t.Fatal("delete_on_destroy must be optional and computed")
	}

	absoluteDirectory, ok := response.Schema.Attributes["absolute_directory"].(resourceschema.StringAttribute)
	if !ok || !absoluteDirectory.Computed {
		t.Fatal("absolute_directory must be a computed string")
	}
}

func TestDirectoryPrivacyUserDeletePreservesRemoteUserByDefault(t *testing.T) {
	t.Parallel()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewDirectoryPrivacyUserResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &DirectoryPrivacyUserResourceModel{
		Directory:         types.StringValue("public_html/private"),
		Username:          types.StringValue("managed"),
		Password:          types.StringNull(),
		PasswordVersion:   types.Int64Value(1),
		DeleteOnDestroy:   types.BoolValue(false),
		AbsoluteDirectory: types.StringValue("/home/test/public_html/private"),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.DeleteResponse{}
	(&directoryPrivacyUserResource{}).Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if response.Diagnostics.WarningsCount() != 1 {
		t.Fatalf(
			"Delete() warnings = %d; want 1",
			response.Diagnostics.WarningsCount(),
		)
	}
}

func TestAccDirectoryPrivacyUserResource(t *testing.T) {
	const (
		resourceName = "cpanel_directory_privacy_user.test"
		passwordOne  = "Directory-User-One-2026!"
		passwordTwo  = "Directory-User-Two-2026!"
	)

	initialDirectory := testAccDirectoryPrivacyDirectory("userresource")
	replacementDirectory := testAccDirectoryPrivacyDirectory("userreplacement")
	initialUsername := testAccDirectoryPrivacyUsername("initial")
	replacementUsername := testAccDirectoryPrivacyUsername("replacement")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCreateDirectory(t, initialDirectory)
			testAccCreateDirectory(t, replacementDirectory)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDirectoryPrivacyUsersDestroyed(
			cpaneldirectoryprivacy.UserDefinition{
				Directory: initialDirectory,
				Username:  initialUsername,
			},
			cpaneldirectoryprivacy.UserDefinition{
				Directory: initialDirectory,
				Username:  replacementUsername,
			},
			cpaneldirectoryprivacy.UserDefinition{
				Directory: replacementDirectory,
				Username:  replacementUsername,
			},
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					initialDirectory,
					initialUsername,
					passwordOne,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"directory",
						initialDirectory,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"username",
						initialUsername,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordOne)),
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_directory",
					),
					testAccCheckDirectoryPrivacyUserExists(
						cpaneldirectoryprivacy.UserDefinition{
							Directory: initialDirectory,
							Username:  initialUsername,
						},
					),
				),
			},
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					initialDirectory,
					initialUsername,
					passwordTwo,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						fmt.Sprintf("%d", testAccPasswordVersion(passwordTwo)),
					),
					testAccCheckDirectoryPrivacyUserExists(
						cpaneldirectoryprivacy.UserDefinition{
							Directory: initialDirectory,
							Username:  initialUsername,
						},
					),
				),
			},
			{
				ResourceName: resourceName,
				ImportStateId: fmt.Sprintf(
					"%s|%s",
					initialDirectory,
					initialUsername,
				),
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "username",
				ImportStateVerifyIgnore: []string{
					"password",
					"password_version",
					"delete_on_destroy",
				},
			},
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					initialDirectory,
					initialUsername,
					passwordTwo,
				),
				Check: testAccCheckDirectoryPrivacyUserExists(
					cpaneldirectoryprivacy.UserDefinition{
						Directory: initialDirectory,
						Username:  initialUsername,
					},
				),
			},
			{
				PreConfig: func() {
					testAccDeleteDirectoryPrivacyUser(
						t,
						initialDirectory,
						initialUsername,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					initialDirectory,
					initialUsername,
					passwordTwo,
				),
				Check: testAccCheckDirectoryPrivacyUserExists(
					cpaneldirectoryprivacy.UserDefinition{
						Directory: initialDirectory,
						Username:  initialUsername,
					},
				),
			},
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					initialDirectory,
					replacementUsername,
					passwordTwo,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDirectoryPrivacyUserMissing(
						initialDirectory,
						initialUsername,
					),
					testAccCheckDirectoryPrivacyUserExists(
						cpaneldirectoryprivacy.UserDefinition{
							Directory: initialDirectory,
							Username:  replacementUsername,
						},
					),
				),
			},
			{
				Config: testAccDirectoryPrivacyUserResourceConfig(
					replacementDirectory,
					replacementUsername,
					passwordTwo,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckDirectoryPrivacyUserMissing(
						initialDirectory,
						replacementUsername,
					),
					testAccCheckDirectoryPrivacyDisabled(initialDirectory),
					testAccCheckDirectoryPrivacyUserExists(
						cpaneldirectoryprivacy.UserDefinition{
							Directory: replacementDirectory,
							Username:  replacementUsername,
						},
					),
				),
			},
		},
	})
}

func testAccDirectoryPrivacyUserResourceConfig(
	directory string,
	username string,
	password string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_directory_privacy" "fixture" {
  directory = %q
  auth_name = "Terraform authorized users"
}

resource "cpanel_directory_privacy_user" "test" {
  directory         = cpanel_directory_privacy.fixture.directory
  username          = %q
  password          = %q
  password_version  = %d
  delete_on_destroy = true
}
`, directory, username, password, testAccPasswordVersion(password))
}
