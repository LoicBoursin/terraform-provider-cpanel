package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelssh "terraform-provider-cpanel/internal/cpanel/ssh"
)

func TestSSHPublicKeyDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewSSHPublicKeyDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	name, ok := response.Schema.Attributes["name"].(datasourceschema.StringAttribute)
	if !ok || !name.Required {
		t.Fatal("name must be a required string")
	}
	publicKey, ok := response.Schema.Attributes["public_key"].(datasourceschema.StringAttribute)
	if !ok || !publicKey.Computed || publicKey.Sensitive {
		t.Fatal("public_key must be a non-sensitive computed string")
	}
}

func TestSSHPublicKeysDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewSSHPublicKeysDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	keys, ok := response.Schema.Attributes["keys"].(datasourceschema.ListNestedAttribute)
	if !ok || !keys.Computed {
		t.Fatal("keys must be a computed nested list")
	}
	if len(keys.NestedObject.Attributes) != 4 {
		t.Fatalf(
			"keys nested attribute count = %d",
			len(keys.NestedObject.Attributes),
		)
	}
}

func TestAccSSHPublicKeyDataSources(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadSSHPublicKeys(t)
	t.Cleanup(func() {
		testAccRequireSSHPublicKeys(t, baseline)
	})

	const (
		inventoryName = "data.cpanel_ssh_public_keys.all"
		keyName       = "data.cpanel_ssh_public_key.selected"
	)
	config := providerConfig + `
data "cpanel_ssh_public_keys" "all" {}
`
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			inventoryName,
			"keys.#",
			strconv.Itoa(len(baseline.metadata)),
		),
		testAccCheckSSHPublicKeyInventoryState(
			inventoryName,
			baseline,
		),
	}
	if len(baseline.metadata) > 0 {
		selected := baseline.keys[baseline.metadata[0].Name]
		config += fmt.Sprintf(`
data "cpanel_ssh_public_key" "selected" {
  name = %q
}
`, selected.Name)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				keyName,
				"name",
				selected.Name,
			),
			testresource.TestCheckResourceAttr(
				keyName,
				"authorized",
				strconv.FormatBool(selected.Authorized),
			),
			testresource.TestCheckResourceAttr(
				keyName,
				"fingerprint_sha256",
				selected.FingerprintSHA256,
			),
			testresource.TestCheckResourceAttr(
				keyName,
				"key_type",
				selected.KeyType,
			),
			testresource.TestCheckResourceAttrSet(
				keyName,
				"public_key",
			),
		)
	}
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireSSHPublicKeys(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{{
			Config: config,
			Check:  testresource.ComposeAggregateTestCheckFunc(checks...),
		}},
	})
}

type testAccSSHPublicKeyBaseline struct {
	metadata []cpanelssh.Metadata
	keys     map[string]cpanelssh.PublicKey
}

func testAccReadSSHPublicKeys(
	t *testing.T,
) testAccSSHPublicKeyBaseline {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	sshClient := cpanelssh.NewClient(client)
	metadata, err := sshClient.List(ctx)
	if err != nil {
		t.Fatalf("list public SSH keys: %v", err)
	}
	baseline := testAccSSHPublicKeyBaseline{
		metadata: append([]cpanelssh.Metadata(nil), metadata...),
		keys:     make(map[string]cpanelssh.PublicKey, len(metadata)),
	}
	for _, item := range metadata {
		key, err := sshClient.Get(ctx, item.Name)
		if err != nil {
			t.Fatalf("read public SSH key %q: %v", item.Name, err)
		}
		if key == nil {
			t.Fatalf("public SSH key %q disappeared", item.Name)
		}
		baseline.keys[item.Name] = *key
	}

	return baseline
}

func testAccRequireSSHPublicKeys(
	t *testing.T,
	expected testAccSSHPublicKeyBaseline,
) {
	t.Helper()

	actual := testAccReadSSHPublicKeys(t)
	if len(actual.metadata) != len(expected.metadata) {
		t.Fatalf(
			"public SSH-key count changed: got %d, expected %d",
			len(actual.metadata),
			len(expected.metadata),
		)
	}
	for index, expectedMetadata := range expected.metadata {
		actualMetadata := actual.metadata[index]
		if !reflect.DeepEqual(actualMetadata, expectedMetadata) {
			t.Fatalf(
				"public SSH-key metadata changed at index %d: got name %q, expected %q",
				index,
				actualMetadata.Name,
				expectedMetadata.Name,
			)
		}
		actualKey, exists := actual.keys[expectedMetadata.Name]
		if !exists {
			t.Fatalf(
				"public SSH key %q disappeared",
				expectedMetadata.Name,
			)
		}
		expectedKey := expected.keys[expectedMetadata.Name]
		if actualKey.FingerprintSHA256 != expectedKey.FingerprintSHA256 ||
			actualKey.ContentSHA256 != expectedKey.ContentSHA256 ||
			actualKey.KeyType != expectedKey.KeyType {
			t.Fatalf(
				"public SSH key %q cryptographic identity changed",
				expectedMetadata.Name,
			)
		}
	}
}

func testAccCheckSSHPublicKeyInventoryState(
	resourceName string,
	expected testAccSSHPublicKeyBaseline,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		instance, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not found in Terraform state", resourceName)
		}
		for index, item := range expected.metadata {
			prefix := fmt.Sprintf("keys.%d.", index)
			if actual := instance.Primary.Attributes[prefix+"name"]; actual != item.Name {
				return fmt.Errorf(
					"%sname = %q; expected %q",
					prefix,
					actual,
					item.Name,
				)
			}
			if actual := instance.Primary.Attributes[prefix+"authorized"]; actual != strconv.FormatBool(item.Authorized) {
				return fmt.Errorf(
					"%sauthorized = %q; expected %t",
					prefix,
					actual,
					item.Authorized,
				)
			}
			if actual := instance.Primary.Attributes[prefix+"created_at"]; actual != strconv.FormatInt(item.CreatedAt, 10) {
				return fmt.Errorf(
					"%screated_at = %q; expected %d",
					prefix,
					actual,
					item.CreatedAt,
				)
			}
			if actual := instance.Primary.Attributes[prefix+"modified_at"]; actual != strconv.FormatInt(item.ModifiedAt, 10) {
				return fmt.Errorf(
					"%smodified_at = %q; expected %d",
					prefix,
					actual,
					item.ModifiedAt,
				)
			}
		}

		return nil
	}
}
