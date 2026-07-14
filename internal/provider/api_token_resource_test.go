package provider

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAPITokenResourceSchemaProtectsSecret(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewAPITokenResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	tokenAttribute, ok := response.Schema.Attributes["token"].(resourceschema.StringAttribute)
	if !ok {
		t.Fatalf(
			"token has type %T, want schema.StringAttribute",
			response.Schema.Attributes["token"],
		)
	}
	if !tokenAttribute.Sensitive || !tokenAttribute.Computed {
		t.Fatal("token must be sensitive and computed")
	}
}

func TestAccAPITokenResource(t *testing.T) {
	const resourceName = "cpanel_api_token.test"

	initialName := testAccAPITokenName("resource")
	renamedName := testAccAPITokenName("renamed")
	expiresAt := time.Now().Add(24 * time.Hour).Unix()

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckAPITokensDestroyed(
			initialName,
			renamedName,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccAPITokenResourceConfig(initialName, 0),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"expires_at",
						"0",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"created_at",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"has_full_access",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"features.#",
						"0",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"whitelist_ips.#",
						"0",
					),
					testAccCheckAPITokenExists(initialName, 0),
				),
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, 0),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						renamedName,
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testAccCheckAPITokenExists(renamedName, 0),
					testAccCheckAPITokensDestroyed(initialName),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        renamedName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"token"},
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, expiresAt),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"expires_at",
						strconv.FormatInt(expiresAt, 10),
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"token",
					),
					testAccCheckAPITokenExists(renamedName, expiresAt),
				),
			},
			{
				PreConfig: func() {
					testAccRevokeAPIToken(t, renamedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccAPITokenResourceConfig(renamedName, expiresAt),
				Check:  testAccCheckAPITokenExists(renamedName, expiresAt),
			},
		},
	})
}

func testAccAPITokenResourceConfig(name string, expiresAt int64) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_api_token" "test" {
  name       = %q
  expires_at = %d
}
`, name, expiresAt)
}
