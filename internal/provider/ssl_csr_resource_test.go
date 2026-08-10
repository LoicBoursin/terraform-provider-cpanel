package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

func TestAccSSLCSRResource(t *testing.T) {
	keyID := testAccSSLCSRKeyID(t)
	suffix := testAccSSLCSRSuffix()
	initial := testAccSSLCSRDefinition(
		keyID,
		testAccSSLCSRName("initial", suffix),
		testAccSSLCSRDomain("initial", suffix),
	)
	renamed := initial
	renamed.FriendlyName = testAccSSLCSRName("renamed", suffix)
	driftedName := testAccSSLCSRName("drift", suffix)
	replacement := testAccSSLCSRDefinition(
		keyID,
		testAccSSLCSRName("replacement", suffix),
		testAccSSLCSRDomain("replacement", suffix),
	)

	var initialID string
	var initialFingerprint string
	var recreatedID string

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSSLCSRsDestroyed(
			initial.FriendlyName,
			renamed.FriendlyName,
			driftedName,
			replacement.FriendlyName,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccSSLCSRResourceConfig(initial, true),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRState(
						initial,
						true,
					),
					testAccCheckSSLCSRExists(
						initial,
						&initialID,
						&initialFingerprint,
					),
				),
			},
			{
				Config: testAccSSLCSRResourceConfig(initial, true),
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: testAccSSLCSRResourceConfig(renamed, true),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRExists(renamed, nil, nil),
					testAccCheckSSLCSRIdentity(
						renamed.FriendlyName,
						&initialID,
						&initialFingerprint,
					),
					testAccCheckSSLCSRsDestroyed(
						initial.FriendlyName,
					),
				),
			},
			{
				PreConfig: func() {
					testAccRenameSSLCSR(
						t,
						renamed.FriendlyName,
						driftedName,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSSLCSRResourceConfig(renamed, true),
				Check:  testAccCheckSSLCSRExists(renamed, nil, nil),
			},
			{
				PreConfig: func() {
					testAccDeleteSSLCSR(t, renamed.FriendlyName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSSLCSRResourceConfig(renamed, true),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRExists(
						renamed,
						&recreatedID,
						nil,
					),
				),
			},
			{
				Config: testAccSSLCSRResourceConfig(
					replacement,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRState(
						replacement,
						true,
					),
					testAccCheckSSLCSRExists(
						replacement,
						nil,
						nil,
					),
					testAccCheckSSLCSRIDMissing(&recreatedID),
					testAccCheckSSLCSRsDestroyed(
						renamed.FriendlyName,
					),
				),
			},
		},
	})
}

func TestAccSSLCSRResourceImport(t *testing.T) {
	const resourceName = "cpanel_ssl_csr.test"

	keyID := testAccSSLCSRKeyID(t)
	suffix := testAccSSLCSRSuffix()
	initial := testAccSSLCSRDefinition(
		keyID,
		testAccSSLCSRName("import", suffix),
		testAccSSLCSRDomain("import", suffix),
	)
	renamed := initial
	renamed.FriendlyName = testAccSSLCSRName("import-renamed", suffix)
	imported := testAccCreateSSLCSR(t, initial)
	importedID := imported.ID
	t.Cleanup(func() {
		testAccDeleteSSLCSR(t, initial.FriendlyName)
		testAccDeleteSSLCSR(t, renamed.FriendlyName)
	})

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSSLCSRsDestroyed(
			initial.FriendlyName,
			renamed.FriendlyName,
		),
		Steps: []testresource.TestStep{
			{
				Config:             testAccSSLCSRResourceConfig(initial, false),
				ResourceName:       resourceName,
				ImportStateId:      importedID,
				ImportState:        true,
				ImportStatePersist: true,
			},
			{
				Config: testAccSSLCSRResourceConfig(initial, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("key_id"),
						knownvalue.Null(),
					),
				},
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRState(initial, false),
					testAccCheckSSLCSRExists(initial, nil, nil),
					testAccCheckSSLCSRIdentity(
						initial.FriendlyName,
						&importedID,
						nil,
					),
				),
			},
			{
				Config: testAccSSLCSRResourceConfig(renamed, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("key_id"),
						knownvalue.Null(),
					),
				},
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSSLCSRState(
						renamed,
						false,
					),
					testAccCheckSSLCSRExists(renamed, nil, nil),
					testAccCheckSSLCSRIdentity(
						renamed.FriendlyName,
						&importedID,
						nil,
					),
				),
			},
		},
	})
}

func TestAccSSLCSRDataSource(t *testing.T) {
	const (
		resourceName   = "cpanel_ssl_csr.test"
		dataSourceName = "data.cpanel_ssl_csr.test"
	)

	suffix := testAccSSLCSRSuffix()
	definition := testAccSSLCSRDefinition(
		testAccSSLCSRKeyID(t),
		testAccSSLCSRName("data", suffix),
		testAccSSLCSRDomain("data", suffix),
	)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSSLCSRsDestroyed(
			definition.FriendlyName,
		),
		Steps: []testresource.TestStep{{
			Config: testAccSSLCSRDataSourceConfig(definition),
			Check: testresource.ComposeAggregateTestCheckFunc(
				testAccCheckSSLCSRState(
					definition,
					true,
				),
				testresource.TestCheckResourceAttrPair(
					dataSourceName,
					"id",
					resourceName,
					"id",
				),
				testresource.TestCheckResourceAttrPair(
					dataSourceName,
					"csr",
					resourceName,
					"csr",
				),
				testresource.TestCheckResourceAttrPair(
					dataSourceName,
					"fingerprint_sha256",
					resourceName,
					"fingerprint_sha256",
				),
				testresource.TestCheckResourceAttr(
					dataSourceName,
					"friendly_name",
					definition.FriendlyName,
				),
				testresource.TestCheckResourceAttr(
					dataSourceName,
					"common_name",
					definition.Domains[0],
				),
				testresource.TestCheckResourceAttr(
					dataSourceName,
					"key_algorithm",
					"rsaEncryption",
				),
			),
		}},
	})
}

func testAccCheckSSLCSRState(
	definition sslcsr.Definition,
	withKeyID bool,
) testresource.TestCheckFunc {
	const resourceName = "cpanel_ssl_csr.test"

	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			resourceName,
			"friendly_name",
			definition.FriendlyName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"domains.#",
			fmt.Sprintf("%d", len(definition.Domains)),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"domains.0",
			definition.Domains[0],
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"country_name",
			definition.CountryName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"state_or_province_name",
			definition.StateOrProvinceName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"locality_name",
			definition.LocalityName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"organization_name",
			definition.OrganizationName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"organizational_unit_name",
			definition.OrganizationalUnitName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"email_address",
			definition.EmailAddress,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"common_name",
			definition.Domains[0],
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"key_algorithm",
			"rsaEncryption",
		),
		testresource.TestCheckResourceAttrSet(resourceName, "id"),
		testresource.TestCheckResourceAttrSet(
			resourceName,
			"fingerprint_sha256",
		),
		testresource.TestCheckResourceAttrSet(resourceName, "csr"),
		testresource.TestCheckResourceAttrSet(resourceName, "created"),
		testresource.TestCheckResourceAttrSet(resourceName, "modulus"),
	}
	if withKeyID {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				"key_id",
				definition.KeyID,
			),
		)
	}

	return testresource.ComposeAggregateTestCheckFunc(checks...)
}

func testAccSSLCSRResourceConfig(
	definition sslcsr.Definition,
	withKeyID bool,
) string {
	keyID := ""
	if withKeyID {
		keyID = fmt.Sprintf("  key_id = %q\n", definition.KeyID)
	}

	return providerConfig + fmt.Sprintf(`
resource "cpanel_ssl_csr" "test" {
%s  friendly_name           = %q
  domains                 = %s
  country_name            = %q
  state_or_province_name  = %q
  locality_name           = %q
  organization_name       = %q
  organizational_unit_name = %q
  email_address           = %q
}
`,
		keyID,
		definition.FriendlyName,
		testAccHCLStringList(definition.Domains),
		definition.CountryName,
		definition.StateOrProvinceName,
		definition.LocalityName,
		definition.OrganizationName,
		definition.OrganizationalUnitName,
		definition.EmailAddress,
	)
}

func testAccSSLCSRDataSourceConfig(
	definition sslcsr.Definition,
) string {
	return testAccSSLCSRResourceConfig(definition, true) + `
data "cpanel_ssl_csr" "test" {
  id = cpanel_ssl_csr.test.id
}
`
}

func testAccSSLCSRDefinition(
	keyID string,
	friendlyName string,
	commonName string,
) sslcsr.Definition {
	return sslcsr.Definition{
		KeyID:                  keyID,
		FriendlyName:           friendlyName,
		Domains:                []string{commonName, "www." + commonName},
		CountryName:            "FR",
		StateOrProvinceName:    "Ile-de-France",
		LocalityName:           "Paris",
		OrganizationName:       "Terraform Provider cPanel",
		OrganizationalUnitName: "Acceptance",
		EmailAddress:           "csr@example.test",
	}
}

func testAccSSLCSRName(kind, suffix string) string {
	return testAccRegisterArtifact(
		fmt.Sprintf("tfcpanelcsr-%s-%s", kind, suffix),
	)
}

func testAccSSLCSRDomain(kind, suffix string) string {
	return testAccRegisterArtifact(
		fmt.Sprintf("tfcpanelcsr-%s-%s.example.test", kind, suffix),
	)
}

func testAccSSLCSRSuffix() string {
	return strings.ToLower(
		acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
	)
}

func testAccHCLStringList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, fmt.Sprintf("%q", value))
	}

	return "[" + strings.Join(quoted, ", ") + "]"
}

func testAccSSLCSRKeyID(t *testing.T) string {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	configured := os.Getenv("CPANEL_TEST_SSL_KEY_ID")
	if configured != "" {
		if err := sslcsr.ValidateID(configured); err != nil {
			t.Fatalf("invalid CPANEL_TEST_SSL_KEY_ID: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	var response struct {
		Data []struct {
			ID           json.RawMessage `json:"id"`
			KeyAlgorithm json.RawMessage `json:"key_algorithm"`
			Modulus      json.RawMessage `json:"modulus"`
		} `json:"data"`
	}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		"SSL",
		"list_keys",
		map[string]string{},
		&response,
	); err != nil {
		t.Fatalf("list cPanel SSL keys: %v", err)
	}

	ids := make([]string, 0, len(response.Data))
	for _, key := range response.Data {
		algorithm, err := testAccSSLCSRJSONScalarString(
			key.KeyAlgorithm,
		)
		if err != nil || algorithm != "rsaEncryption" {
			continue
		}
		modulus, err := testAccSSLCSRJSONScalarString(key.Modulus)
		if err != nil || modulus == "" {
			continue
		}
		id, err := testAccSSLCSRJSONIdentifier(key.ID)
		if err != nil || sslcsr.ValidateID(id) != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		t.Fatal(
			"the cPanel acceptance account has no usable RSA SSL key",
		)
	}
	sort.Strings(ids)
	if configured != "" {
		index := sort.SearchStrings(ids, configured)
		if index == len(ids) || ids[index] != configured {
			t.Fatalf(
				"CPANEL_TEST_SSL_KEY_ID %q is not a usable RSA SSL key",
				configured,
			)
		}

		return configured
	}

	return ids[0]
}

func testAccSSLCSRJSONScalarString(
	raw json.RawMessage,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", err
	}

	return result, nil
}

func testAccSSLCSRJSONIdentifier(
	raw json.RawMessage,
) (string, error) {
	if value, err := testAccSSLCSRJSONScalarString(raw); err == nil {
		return value, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value json.Number
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}

	return value.String(), nil
}

func testAccCheckSSLCSRExists(
	definition sslcsr.Definition,
	capturedID *string,
	capturedFingerprint *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		csr, err := testAccGetSSLCSRByFriendlyName(
			definition.FriendlyName,
		)
		if err != nil {
			return err
		}
		if csr == nil {
			return fmt.Errorf(
				"SSL CSR %q was not found",
				definition.FriendlyName,
			)
		}
		if err := sslcsr.VerifyDefinition(*csr, definition); err != nil {
			return err
		}
		if csr.KeyAlgorithm != "rsaEncryption" ||
			csr.Modulus == "" ||
			csr.ECDSACurveName != "" ||
			csr.ECDSAPublic != "" {
			return fmt.Errorf(
				"SSL CSR %q has unexpected key metadata",
				definition.FriendlyName,
			)
		}
		if csr.Created <= 0 {
			return fmt.Errorf(
				"SSL CSR %q has invalid creation time %d",
				definition.FriendlyName,
				csr.Created,
			)
		}
		if capturedID != nil {
			*capturedID = csr.ID
		}
		if capturedFingerprint != nil {
			*capturedFingerprint = csr.FingerprintSHA256
		}

		return nil
	}
}

func testAccCheckSSLCSRIdentity(
	friendlyName string,
	expectedID *string,
	expectedFingerprint *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		csr, err := testAccGetSSLCSRByFriendlyName(friendlyName)
		if err != nil {
			return err
		}
		if csr == nil {
			return fmt.Errorf("SSL CSR %q was not found", friendlyName)
		}
		if expectedID != nil {
			if *expectedID == "" {
				return fmt.Errorf("expected SSL CSR id was not captured")
			}
			if csr.ID != *expectedID {
				return fmt.Errorf(
					"SSL CSR %q id is %q; expected %q",
					friendlyName,
					csr.ID,
					*expectedID,
				)
			}
		}
		if expectedFingerprint != nil {
			if *expectedFingerprint == "" {
				return fmt.Errorf(
					"expected SSL CSR fingerprint was not captured",
				)
			}
			if csr.FingerprintSHA256 != *expectedFingerprint {
				return fmt.Errorf(
					"SSL CSR %q fingerprint changed",
					friendlyName,
				)
			}
		}

		return nil
	}
}

func testAccCheckSSLCSRIDMissing(
	id *string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if id == nil || *id == "" {
			return fmt.Errorf("SSL CSR id to verify was not captured")
		}
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()
		client, err := testAccClient()
		if err != nil {
			return err
		}
		csr, err := sslcsr.NewClient(client).Get(ctx, *id)
		if err != nil {
			return err
		}
		if csr != nil {
			return fmt.Errorf("SSL CSR id %q still exists", *id)
		}

		return nil
	}
}

func testAccCheckSSLCSRsDestroyed(
	friendlyNames ...string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()
		client, err := testAccClient()
		if err != nil {
			return err
		}
		csrs, err := sslcsr.NewClient(client).List(ctx)
		if err != nil {
			return err
		}
		for _, csr := range csrs {
			for _, friendlyName := range friendlyNames {
				if csr.FriendlyName == friendlyName {
					return fmt.Errorf(
						"SSL CSR %q still exists",
						friendlyName,
					)
				}
			}
		}

		return nil
	}
}

func testAccRenameSSLCSR(
	t *testing.T,
	currentFriendlyName string,
	newFriendlyName string,
) {
	t.Helper()

	current, err := testAccGetSSLCSRByFriendlyName(currentFriendlyName)
	if err != nil {
		t.Fatalf("read SSL CSR %q: %v", currentFriendlyName, err)
	}
	if current == nil {
		t.Fatalf("SSL CSR %q was not found", currentFriendlyName)
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := sslcsr.NewClient(client).Rename(
		ctx,
		current.Identity(),
		currentFriendlyName,
		newFriendlyName,
	); err != nil {
		t.Fatalf(
			"rename SSL CSR %q: %v",
			currentFriendlyName,
			err,
		)
	}
}

func testAccDeleteSSLCSR(t *testing.T, friendlyName string) {
	t.Helper()

	current, err := testAccGetSSLCSRByFriendlyName(friendlyName)
	if err != nil {
		t.Fatalf("read SSL CSR %q: %v", friendlyName, err)
	}
	if current == nil {
		return
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := sslcsr.NewClient(client).Delete(
		ctx,
		current.Identity(),
	); err != nil {
		t.Fatalf("delete SSL CSR %q: %v", friendlyName, err)
	}
}

func testAccCreateSSLCSR(
	t *testing.T,
	definition sslcsr.Definition,
) *sslcsr.CSR {
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
	generated, err := sslcsr.NewClient(client).Generate(ctx, definition)
	if err != nil {
		t.Fatalf(
			"generate SSL CSR %q: %v",
			definition.FriendlyName,
			err,
		)
	}

	return generated
}

func testAccGetSSLCSRByFriendlyName(
	friendlyName string,
) (*sslcsr.CSR, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		return nil, err
	}
	csrs, err := sslcsr.NewClient(client).List(ctx)
	if err != nil {
		return nil, err
	}

	var match *sslcsr.CSR
	for index := range csrs {
		if csrs[index].FriendlyName != friendlyName {
			continue
		}
		if match != nil {
			return nil, fmt.Errorf(
				"multiple SSL CSRs use friendly name %q",
				friendlyName,
			)
		}
		current := csrs[index]
		match = &current
	}

	return match, nil
}
