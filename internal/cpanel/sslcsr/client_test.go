package sslcsr

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientListsCanonicalCSRsSortedByID(t *testing.T) {
	t.Parallel()

	firstDefinition := testDefinition()
	firstDefinition.FriendlyName = "Numeric CSR"
	firstPEM := testCSRPEM(t, firstDefinition)
	secondDefinition := testDefinition()
	secondDefinition.FriendlyName = "Beta CSR"
	secondDefinition.Domains = []string{
		"beta.example.test",
		"www.beta.example.test",
	}
	secondDefinition.OrganizationName = "Beta Organization"
	secondPEM := testCSRPEM(t, secondDefinition)

	secondInventory := inventoryItem(
		secondDefinition,
		"csr-b",
		1700000010,
	)
	firstInventory := inventoryItem(
		firstDefinition,
		"123",
		"1700000000",
	)
	firstInventory["id"] = 123

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				secondInventory,
				firstInventory,
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"123"}},
			response: successResponse(showData(
				firstDefinition,
				"123",
				1700000000,
				firstPEM,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-b"}},
			response: successResponse(showData(
				secondDefinition,
				"csr-b",
				1700000010,
				secondPEM,
			)),
		},
	})

	csrs, err := client.List(t.Context())
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(csrs) != 2 {
		t.Fatalf("len(csrs) = %d, want 2", len(csrs))
	}
	if csrs[0].ID != "123" || csrs[1].ID != "csr-b" {
		t.Fatalf("CSR order = %q, %q", csrs[0].ID, csrs[1].ID)
	}

	firstParsed, err := ParsePEM(firstPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if csrs[0].FriendlyName != firstDefinition.FriendlyName ||
		csrs[0].CSRPEM != firstParsed.NormalizedPEM ||
		csrs[0].FingerprintSHA256 != firstParsed.SHA256Fingerprint ||
		csrs[0].Created != 1700000000 ||
		csrs[0].CommonName != firstDefinition.Domains[0] ||
		csrs[0].CountryName != firstDefinition.CountryName ||
		csrs[0].StateOrProvinceName !=
			firstDefinition.StateOrProvinceName ||
		csrs[0].LocalityName != firstDefinition.LocalityName ||
		csrs[0].OrganizationName != firstDefinition.OrganizationName ||
		csrs[0].OrganizationalUnitName !=
			firstDefinition.OrganizationalUnitName ||
		csrs[0].EmailAddress != firstDefinition.EmailAddress ||
		!reflect.DeepEqual(
			csrs[0].Domains,
			[]string{"example.test", "www.example.test"},
		) {
		t.Fatalf("first CSR = %#v", csrs[0])
	}
}

func TestClientGetReturnsNilWhenCSRIsAbsent(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	client := newScriptedClient(t, []scriptedRequest{{
		method: http.MethodGet,
		path:   "/execute/SSL/list_csrs",
		response: successResponse([]map[string]any{
			inventoryItem(definition, "other-id", 1700000000),
		}),
	}})

	csr, err := client.Get(t.Context(), "missing-id")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if csr != nil {
		t.Fatalf("Get() = %#v, want nil", csr)
	}
}

func TestClientGetsCSRByIDWithoutShowingUnrelatedEntries(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	otherDefinition := definition
	otherDefinition.FriendlyName = "Other CSR"
	otherDefinition.Domains = []string{"other.example.test"}

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					otherDefinition,
					"other-id",
					1700000001,
				),
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	})

	csr, err := client.Get(t.Context(), "csr-id")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if csr == nil {
		t.Fatal("Get() = nil")
	}
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if csr.ID != "csr-id" ||
		csr.FingerprintSHA256 != parsed.SHA256Fingerprint ||
		csr.CSRPEM != parsed.NormalizedPEM {
		t.Fatalf("CSR = %#v", csr)
	}
}

func TestClientGetsECDSACSRWhenShowOmitsECDSAFields(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	show := showData(
		definition,
		"csr-id",
		1700000000,
		csrPEM,
	)
	details, ok := show["details"].(map[string]any)
	if !ok {
		t.Fatalf("show details type = %T", show["details"])
	}
	delete(details, "ecdsa_curve_name")
	delete(details, "ecdsa_public")

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/show_csr",
			values:   url.Values{"id": {"csr-id"}},
			response: successResponse(show),
		},
	})

	csr, err := client.Get(t.Context(), "csr-id")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if csr == nil ||
		csr.ECDSACurveName != "prime256v1" ||
		csr.ECDSAPublic != testECDSAPublicHex() {
		t.Fatalf("CSR = %#v", csr)
	}
}

func TestClientRejectsCSRWhoseInventoryKeyDoesNotMatchPKCS10(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	wrongPrivateKey := testECDSAPrivateKeyFromScalar(2)
	wrongPublicKey, ok := wrongPrivateKey.Public().(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf(
			"wrong private key public key type = %T",
			wrongPrivateKey.Public(),
		)
	}
	wrongKey := publicKeyMetadata{
		KeyAlgorithm:   "id-ecPublicKey",
		ECDSACurveName: "prime256v1",
		ECDSAPublic:    testCompressedECDSAPublic(wrongPublicKey),
	}
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					wrongKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				definition,
				csrPEM,
				wrongKey,
			)),
		},
	})

	if _, err := client.Get(
		t.Context(),
		"csr-id",
	); err == nil ||
		!strings.Contains(err.Error(), "PKCS#10 public key") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestClientFailsClosedOnInvalidInventory(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	valid := inventoryItem(definition, "csr-id", 1700000000)
	duplicate := inventoryItem(definition, "csr-id", 1700000001)
	missingID := inventoryItem(definition, "csr-id", 1700000000)
	delete(missingID, "id")
	missingCommonName := inventoryItem(
		definition,
		"csr-id",
		1700000000,
	)
	delete(missingCommonName, "commonName")
	duplicateDomains := inventoryItem(
		definition,
		"csr-id",
		1700000000,
	)
	duplicateDomains["domains"] = []string{
		"example.test",
		"example.test",
	}
	malformedCreated := inventoryItem(
		definition,
		"csr-id",
		1700000000,
	)
	malformedCreated["created"] = "not-an-integer"
	numericDomain := inventoryItem(
		definition,
		"csr-id",
		1700000000,
	)
	numericDomain["domains"] = []any{"example.test", 123}
	numericKeyAlgorithm := inventoryItem(
		definition,
		"csr-id",
		1700000000,
	)
	numericKeyAlgorithm["key_algorithm"] = 123

	testCases := map[string]map[string]any{
		"missing data": {
			"status": 1,
		},
		"null data": {
			"status": 1,
			"data":   nil,
		},
		"non-array data": {
			"status": 1,
			"data":   map[string]any{},
		},
		"duplicate id": successResponse([]map[string]any{
			valid,
			duplicate,
		}),
		"missing id": successResponse([]map[string]any{
			missingID,
		}),
		"missing common name": successResponse([]map[string]any{
			missingCommonName,
		}),
		"duplicate domains": successResponse([]map[string]any{
			duplicateDomains,
		}),
		"malformed created": successResponse([]map[string]any{
			malformedCreated,
		}),
		"numeric domain": successResponse([]map[string]any{
			numericDomain,
		}),
		"numeric key algorithm": successResponse([]map[string]any{
			numericKeyAlgorithm,
		}),
	}

	for name, response := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newScriptedClient(t, []scriptedRequest{{
				method:   http.MethodGet,
				path:     "/execute/SSL/list_csrs",
				response: response,
			}})
			if _, err := client.List(t.Context()); err == nil {
				t.Fatal("List() returned no error")
			}
		})
	}
}

func TestClientFailsClosedOnInvalidShowResponse(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	validPEM := testCSRPEM(t, definition)
	wrongDefinition := definition
	wrongDefinition.Domains = []string{
		"wrong.example.test",
		"www.wrong.example.test",
	}
	wrongPEM := testCSRPEM(t, wrongDefinition)

	testCases := map[string]any{
		"missing data": nil,
		"missing details": map[string]any{
			"csr": validPEM,
		},
		"wrong details id": map[string]any{
			"csr": validPEM,
			"details": showDetails(
				definition,
				"other-id",
				1700000000,
			),
		},
		"missing PEM": map[string]any{
			"csr": nil,
			"details": showDetails(
				definition,
				"csr-id",
				1700000000,
			),
		},
		"malformed PEM": map[string]any{
			"csr": "not PEM",
			"details": showDetails(
				definition,
				"csr-id",
				1700000000,
			),
		},
		"PEM metadata mismatch": map[string]any{
			"csr": wrongPEM,
			"details": showDetails(
				definition,
				"csr-id",
				1700000000,
			),
		},
	}

	for name, data := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newScriptedClient(t, []scriptedRequest{
				{
					method: http.MethodGet,
					path:   "/execute/SSL/list_csrs",
					response: successResponse([]map[string]any{
						inventoryItem(
							definition,
							"csr-id",
							1700000000,
						),
					}),
				},
				{
					method: http.MethodGet,
					path:   "/execute/SSL/show_csr",
					values: url.Values{"id": {"csr-id"}},
					response: successResponse(
						data,
					),
				},
			})
			if _, err := client.Get(
				t.Context(),
				"csr-id",
			); err == nil {
				t.Fatal("Get() returned no error")
			}
		})
	}
}

func TestClientGeneratesCSRWithExistingKeyAndVerifiesResult(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testRSACSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/generate_csr",
			values: url.Values{
				"countryName": {
					definition.CountryName,
				},
				"domains": {
					strings.Join(definition.Domains, ","),
				},
				"emailAddress": {
					definition.EmailAddress,
				},
				"friendly_name": {
					definition.FriendlyName,
				},
				"key_id": {
					definition.KeyID,
				},
				"localityName": {
					definition.LocalityName,
				},
				"organizationName": {
					definition.OrganizationName,
				},
				"organizationalUnitName": {
					definition.OrganizationalUnitName,
				},
				"stateOrProvinceName": {
					definition.StateOrProvinceName,
				},
			},
			response: successResponse(generatedDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
	})

	csr, err := client.Generate(t.Context(), definition)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if csr.ID != "csr-id" ||
		csr.CSRPEM != parsed.NormalizedPEM ||
		csr.FingerprintSHA256 != parsed.SHA256Fingerprint ||
		csr.FriendlyName != definition.FriendlyName {
		t.Fatalf("CSR = %#v", csr)
	}
}

func TestClientRecoversCSRWhenGenerationResponseIsLost(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method:     http.MethodPost,
			path:       "/execute/SSL/generate_csr",
			values:     generateValues(definition),
			statusCode: http.StatusServiceUnavailable,
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
	})

	csr, err := client.Generate(t.Context(), definition)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if csr.ID != "csr-id" ||
		csr.FriendlyName != definition.FriendlyName {
		t.Fatalf("recovered CSR = %#v", csr)
	}
}

func TestClientPollsUntilDelayedGeneratedCSRAppears(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method:     http.MethodPost,
			path:       "/execute/SSL/generate_csr",
			values:     generateValues(definition),
			statusCode: http.StatusServiceUnavailable,
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
	})
	client.generationRecoveryDelay = time.Millisecond
	client.generationRecoveryLimit = time.Second

	csr, err := client.Generate(t.Context(), definition)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if csr.ID != "csr-id" {
		t.Fatalf("recovered CSR id = %q, want csr-id", csr.ID)
	}
}

func TestClientRecoversCSRWhenGenerationContextIsCanceled(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	ctx, cancel := context.WithCancel(t.Context())
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method:     http.MethodPost,
			path:       "/execute/SSL/generate_csr",
			values:     generateValues(definition),
			statusCode: http.StatusServiceUnavailable,
			beforeSend: cancel,
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
	})

	csr, err := client.Generate(ctx, definition)
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if csr.ID != "csr-id" {
		t.Fatalf("recovered CSR id = %q, want csr-id", csr.ID)
	}
}

func TestClientRejectsGeneratedCSRUsingDifferentSelectedKey(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	selectedPEM := testRSACSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		selectedPEM,
	)
	actualPEM := testRSACSRPEM(t, definition)
	actualKey := testPublicKeyFromCSR(
		t,
		"unexpected-key-id",
		actualPEM,
	)
	actualInventory := inventoryItemWithKey(
		definition,
		actualKey,
	)
	actualShow := showDataWithKey(
		definition,
		actualPEM,
		actualKey,
	)

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/generate_csr",
			values: generateValues(definition),
			response: successResponse(generatedDataWithKey(
				definition,
				actualPEM,
				actualKey,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				actualInventory,
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/show_csr",
			values:   url.Values{"id": {"csr-id"}},
			response: successResponse(actualShow),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				actualInventory,
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/show_csr",
			values:   url.Values{"id": {"csr-id"}},
			response: successResponse(actualShow),
		},
	})

	if _, err := client.Generate(
		t.Context(),
		definition,
	); err == nil ||
		!strings.Contains(err.Error(), "selected key") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestClientRejectsInvalidOrMissingSelectedKeyBeforeMutation(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testRSACSRPEM(t, definition)
	validKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	numericAlgorithm := publicKeyInventoryItem(validKey)
	numericAlgorithm["key_algorithm"] = 123
	invalidModulus := publicKeyInventoryItem(validKey)
	invalidModulus["modulus"] = "not-hexadecimal"
	mixedMetadata := publicKeyInventoryItem(validKey)
	mixedMetadata["ecdsa_curve_name"] = "prime256v1"
	mixedMetadata["ecdsa_public"] = "04abcdef"

	testCases := map[string][]map[string]any{
		"missing selected key": {},
		"duplicate key id": {
			publicKeyInventoryItem(validKey),
			publicKeyInventoryItem(validKey),
		},
		"numeric algorithm": {numericAlgorithm},
		"invalid modulus":   {invalidModulus},
		"mixed metadata":    {mixedMetadata},
	}
	for name, inventory := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newScriptedClient(t, []scriptedRequest{{
				method:   http.MethodGet,
				path:     "/execute/SSL/list_keys",
				response: successResponse(inventory),
			}})
			if _, err := client.Generate(
				t.Context(),
				definition,
			); err == nil {
				t.Fatal("Generate() returned no error")
			}
		})
	}
}

func TestClientRejectsInvalidGenerateDefinitionBeforeRequest(t *testing.T) {
	t.Parallel()

	testCases := map[string]func(*Definition){
		"missing key id": func(definition *Definition) {
			definition.KeyID = ""
		},
		"missing friendly name": func(definition *Definition) {
			definition.FriendlyName = ""
		},
		"missing domains": func(definition *Definition) {
			definition.Domains = nil
		},
		"duplicate domains": func(definition *Definition) {
			definition.Domains = []string{
				"example.test",
				"example.test",
			}
		},
		"invalid country": func(definition *Definition) {
			definition.CountryName = "us"
		},
		"missing organization": func(definition *Definition) {
			definition.OrganizationName = ""
		},
		"invalid email": func(definition *Definition) {
			definition.EmailAddress = "Example <admin@example.test>"
		},
	}

	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			definition := testDefinition()
			mutate(&definition)
			client := newScriptedClient(t, nil)
			if _, err := client.Generate(
				t.Context(),
				definition,
			); err == nil {
				t.Fatal("Generate() returned no error")
			}
		})
	}
}

func TestClientRejectsGeneratedExistingID(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/generate_csr",
			values: generateValues(definition),
			response: successResponse(generatedDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					definition,
					selectedKey,
				),
			}),
		},
	})

	if _, err := client.Generate(
		t.Context(),
		definition,
	); err == nil || !strings.Contains(err.Error(), "existing SSL CSR id") {
		t.Fatalf("Generate() error = %v", err)
	}
}

func TestClientRejectsUnverifiedGenerationResult(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	driftedDefinition := definition
	driftedDefinition.FriendlyName = "Unexpected name"
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_keys",
			response: successResponse([]map[string]any{
				publicKeyInventoryItem(selectedKey),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/generate_csr",
			values: generateValues(definition),
			response: successResponse(generatedDataWithKey(
				definition,
				csrPEM,
				selectedKey,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					driftedDefinition,
					selectedKey,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showDataWithKey(
				driftedDefinition,
				csrPEM,
				selectedKey,
			)),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItemWithKey(
					driftedDefinition,
					selectedKey,
				),
			}),
		},
	})

	if _, err := client.Generate(
		t.Context(),
		definition,
	); err == nil {
		t.Fatal("Generate() returned no error")
	}
}

func TestClientRenamesExactCSRAndVerifiesIdentity(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	renamedDefinition := definition
	renamedDefinition.FriendlyName = "Renamed CSR"

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/set_csr_friendly_name",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {
					"csr-id",
				},
				"new_friendly_name": {
					renamedDefinition.FriendlyName,
				},
			},
			response: successResponse(nil),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					renamedDefinition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				renamedDefinition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	})

	renamed, err := client.Rename(
		t.Context(),
		Identity{
			ID:                "csr-id",
			FingerprintSHA256: parsed.SHA256Fingerprint,
		},
		definition.FriendlyName,
		renamedDefinition.FriendlyName,
	)
	if err != nil {
		t.Fatalf("Rename() error: %v", err)
	}
	if renamed.FriendlyName != renamedDefinition.FriendlyName ||
		renamed.FingerprintSHA256 != parsed.SHA256Fingerprint {
		t.Fatalf("renamed CSR = %#v", renamed)
	}
}

func TestClientRenameRefusesFingerprintMismatchBeforeMutation(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	})

	if _, err := client.Rename(
		t.Context(),
		Identity{
			ID:                "csr-id",
			FingerprintSHA256: strings.Repeat("0", 64),
		},
		definition.FriendlyName,
		"Renamed CSR",
	); err == nil {
		t.Fatal("Rename() returned no error")
	}
}

func TestClientRenameRefusesFriendlyNameMismatchBeforeMutation(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	})

	if _, err := client.Rename(
		t.Context(),
		Identity{
			ID:                "csr-id",
			FingerprintSHA256: parsed.SHA256Fingerprint,
		},
		"Unexpected CSR",
		"Renamed CSR",
	); err == nil {
		t.Fatal("Rename() returned no error")
	}
}

func TestClientRenameRejectsPostMutationIdentityChange(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	beforePEM := testCSRPEM(t, definition)
	afterPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(beforePEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	renamedDefinition := definition
	renamedDefinition.FriendlyName = "Renamed CSR"

	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				beforePEM,
			)),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/set_csr_friendly_name",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {"csr-id"},
				"new_friendly_name": {
					renamedDefinition.FriendlyName,
				},
			},
			response: successResponse(nil),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					renamedDefinition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				renamedDefinition,
				"csr-id",
				1700000000,
				afterPEM,
			)),
		},
	})

	if _, err := client.Rename(
		t.Context(),
		Identity{
			ID:                "csr-id",
			FingerprintSHA256: parsed.SHA256Fingerprint,
		},
		definition.FriendlyName,
		renamedDefinition.FriendlyName,
	); err == nil {
		t.Fatal("Rename() returned no error")
	}
}

func TestClientDeletesOnlyExactCSRAndVerifiesAbsence(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/delete_csr",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {"csr-id"},
			},
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{}),
		},
	})

	if err := client.Delete(t.Context(), Identity{
		ID:                "csr-id",
		FingerprintSHA256: parsed.SHA256Fingerprint,
	}); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestClientDeleteRefusesFingerprintMismatchBeforeMutation(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	})

	if err := client.Delete(t.Context(), Identity{
		ID:                "csr-id",
		FingerprintSHA256: strings.Repeat("0", 64),
	}); err == nil {
		t.Fatal("Delete() returned no error")
	}
}

func TestClientDeleteIsIdempotentWhenCSRIsAbsent(t *testing.T) {
	t.Parallel()

	client := newScriptedClient(t, []scriptedRequest{{
		method:   http.MethodGet,
		path:     "/execute/SSL/list_csrs",
		response: successResponse([]map[string]any{}),
	}})
	if err := client.Delete(t.Context(), Identity{
		ID:                "csr-id",
		FingerprintSHA256: strings.Repeat("0", 64),
	}); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
}

func TestClientDeleteRejectsCSRStillPresentAfterMutation(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	inventory := inventoryItem(definition, "csr-id", 1700000000)
	show := showData(
		definition,
		"csr-id",
		1700000000,
		csrPEM,
	)
	client := newScriptedClient(t, []scriptedRequest{
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{inventory}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/show_csr",
			values:   url.Values{"id": {"csr-id"}},
			response: successResponse(show),
		},
		{
			method: http.MethodPost,
			path:   "/execute/SSL/delete_csr",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {"csr-id"},
			},
			response: successResponse(nil),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{inventory}),
		},
		{
			method:   http.MethodGet,
			path:     "/execute/SSL/show_csr",
			values:   url.Values{"id": {"csr-id"}},
			response: successResponse(show),
		},
	})

	if err := client.Delete(t.Context(), Identity{
		ID:                "csr-id",
		FingerprintSHA256: parsed.SHA256Fingerprint,
	}); err == nil {
		t.Fatal("Delete() returned no error")
	}
}

func TestClientDoesNotRetryMutations(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	selectedKey := testPublicKeyFromCSR(
		t,
		definition.KeyID,
		csrPEM,
	)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	preflight := []scriptedRequest{
		{
			method: http.MethodGet,
			path:   "/execute/SSL/list_csrs",
			response: successResponse([]map[string]any{
				inventoryItem(
					definition,
					"csr-id",
					1700000000,
				),
			}),
		},
		{
			method: http.MethodGet,
			path:   "/execute/SSL/show_csr",
			values: url.Values{"id": {"csr-id"}},
			response: successResponse(showData(
				definition,
				"csr-id",
				1700000000,
				csrPEM,
			)),
		},
	}

	t.Run("generate", func(t *testing.T) {
		t.Parallel()

		client := newScriptedClient(t, []scriptedRequest{
			{
				method: http.MethodGet,
				path:   "/execute/SSL/list_keys",
				response: successResponse([]map[string]any{
					publicKeyInventoryItem(selectedKey),
				}),
			},
			{
				method:   http.MethodGet,
				path:     "/execute/SSL/list_csrs",
				response: successResponse([]map[string]any{}),
			},
			{
				method:     http.MethodPost,
				path:       "/execute/SSL/generate_csr",
				values:     generateValues(definition),
				statusCode: http.StatusServiceUnavailable,
			},
			{
				method:   http.MethodGet,
				path:     "/execute/SSL/list_csrs",
				response: successResponse([]map[string]any{}),
			},
		})
		if _, err := client.Generate(
			t.Context(),
			definition,
		); err == nil {
			t.Fatal("Generate() returned no error")
		}
	})

	t.Run("rename", func(t *testing.T) {
		t.Parallel()

		requests := append([]scriptedRequest(nil), preflight...)
		requests = append(requests, scriptedRequest{
			method: http.MethodPost,
			path:   "/execute/SSL/set_csr_friendly_name",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {"csr-id"},
				"new_friendly_name": {
					"Renamed CSR",
				},
			},
			statusCode: http.StatusServiceUnavailable,
		})
		client := newScriptedClient(t, requests)
		if _, err := client.Rename(
			t.Context(),
			Identity{
				ID:                "csr-id",
				FingerprintSHA256: parsed.SHA256Fingerprint,
			},
			definition.FriendlyName,
			"Renamed CSR",
		); err == nil {
			t.Fatal("Rename() returned no error")
		}
	})

	t.Run("delete", func(t *testing.T) {
		t.Parallel()

		requests := append([]scriptedRequest(nil), preflight...)
		requests = append(requests, scriptedRequest{
			method: http.MethodPost,
			path:   "/execute/SSL/delete_csr",
			values: url.Values{
				"friendly_name": {
					definition.FriendlyName,
				},
				"id": {"csr-id"},
			},
			statusCode: http.StatusServiceUnavailable,
		})
		client := newScriptedClient(t, requests)
		if err := client.Delete(t.Context(), Identity{
			ID:                "csr-id",
			FingerprintSHA256: parsed.SHA256Fingerprint,
		}); err == nil {
			t.Fatal("Delete() returned no error")
		}
	})
}

func TestParsePEMNormalizesAndFingerprintsCSR(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(
		"\n  " + strings.TrimSpace(csrPEM) + "  \n",
	)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if parsed.Request == nil ||
		!strings.HasPrefix(parsed.NormalizedPEM, csrPEMBlockStart) ||
		strings.HasSuffix(parsed.NormalizedPEM, "\n") ||
		len(parsed.SHA256Fingerprint) != 64 {
		t.Fatalf("parsed CSR = %#v", parsed)
	}

	equal, err := EqualPEM(parsed.NormalizedPEM, csrPEM)
	if err != nil {
		t.Fatalf("EqualPEM() error: %v", err)
	}
	if !equal {
		t.Fatal("EqualPEM() = false, want true")
	}
	otherPEM := testCSRPEM(t, definition)
	equal, err = EqualPEM(csrPEM, otherPEM)
	if err != nil {
		t.Fatalf("EqualPEM() error: %v", err)
	}
	if equal {
		t.Fatal("EqualPEM() = true, want false")
	}
}

func TestParsePEMRejectsMalformedOrAdditionalData(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil {
		t.Fatal("pem.Decode() = nil")
	}
	mutatedDER := append([]byte(nil), block.Bytes...)
	mutatedDER[len(mutatedDER)-1] ^= 0xff
	invalidSignature := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: mutatedDER,
	})
	wrongBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: block.Bytes,
	})

	for name, value := range map[string]string{
		"empty":             "",
		"not PEM":           "not PEM",
		"multiple blocks":   csrPEM + csrPEM,
		"trailing data":     csrPEM + "trailing",
		"wrong block":       string(wrongBlock),
		"invalid DER":       string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: []byte("invalid")})),
		"invalid signature": string(invalidSignature),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParsePEM(value); err == nil {
				t.Fatal("ParsePEM() returned no error")
			}
		})
	}
}

func TestAttachPEMRejectsUnsupportedPKCS10Content(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	testURI, err := url.Parse("spiffe://example.test/workload")
	if err != nil {
		t.Fatalf("url.Parse() error: %v", err)
	}
	testCases := map[string]func(*x509.CertificateRequest){
		"IP SAN": func(request *x509.CertificateRequest) {
			request.IPAddresses = []net.IP{net.ParseIP("192.0.2.10")}
		},
		"email SAN": func(request *x509.CertificateRequest) {
			request.EmailAddresses = []string{"hidden@example.test"}
		},
		"URI SAN": func(request *x509.CertificateRequest) {
			request.URIs = []*url.URL{testURI}
		},
		"extra extension": func(request *x509.CertificateRequest) {
			request.ExtraExtensions = []pkix.Extension{{
				Id:    asn1.ObjectIdentifier{1, 2, 3, 4},
				Value: []byte{0x05, 0x00},
			}}
		},
		"extra subject value": func(request *x509.CertificateRequest) {
			request.Subject.ExtraNames = append(
				request.Subject.ExtraNames,
				pkix.AttributeTypeAndValue{
					Type:  asn1.ObjectIdentifier{1, 2, 3, 4},
					Value: "hidden",
				},
			)
		},
		"duplicate country": func(request *x509.CertificateRequest) {
			request.Subject.Country = []string{"US", "FR"}
		},
		"duplicate DNS SAN": func(request *x509.CertificateRequest) {
			request.DNSNames = append(
				request.DNSNames,
				request.DNSNames[0],
			)
		},
		"common name omitted from DNS SAN": func(
			request *x509.CertificateRequest,
		) {
			request.DNSNames = append(
				[]string(nil),
				request.DNSNames[1:]...,
			)
		},
	}

	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			template := testCSRTemplate(definition)
			mutate(template)
			csrPEM := testCSRPEMWithTemplateAndPrivateKey(
				t,
				template,
				testECDSAPrivateKey(),
			)
			if _, err := attachPEM(
				testCSRMetadata(definition),
				csrPEM,
			); err == nil {
				t.Fatal("attachPEM() returned no error")
			}
		})
	}
}

func TestAttachPEMRejectsUnsupportedPKCS10Version(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	attributes := []asn1.RawValue{
		testRawExtensionRequestAttribute(
			t,
			testRawDNSNames(definition.Domains),
		),
	}
	csrPEM, metadata := testRawCSRPEMWithVersion(
		t,
		definition,
		1,
		attributes,
	)
	if _, err := ParsePEM(csrPEM); err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	if _, err := attachPEM(
		metadata,
		csrPEM,
	); err == nil ||
		!strings.Contains(err.Error(), "version is 1; expected 0") {
		t.Fatalf("attachPEM() error = %v", err)
	}
}

func TestAttachPEMRejectsRawPKCS10ContentIgnoredByX509(
	t *testing.T,
) {
	t.Parallel()

	definition := testDefinition()
	dnsNames := testRawDNSNames(definition.Domains)
	otherName := asn1.RawValue{
		Class:      asn1.ClassContextSpecific,
		Tag:        0,
		IsCompound: true,
		Bytes: []byte{
			0x06, 0x02, 0x2a, 0x03,
			0xa0, 0x03, 0x0c, 0x01, 'x',
		},
	}
	extensionRequest := testRawExtensionRequestAttribute(
		t,
		dnsNames,
	)
	extensionRequestWithOtherName := testRawExtensionRequestAttribute(
		t,
		append(
			append([]asn1.RawValue(nil), dnsNames...),
			otherName,
		),
	)
	challengePassword := testRawPKCS10Attribute(
		t,
		asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 7},
		testRawValueFromDER(
			t,
			testASN1MarshalWithParams(
				t,
				"challenge",
				"utf8",
			),
		),
	)

	testCases := map[string]struct {
		attributes []asn1.RawValue
		want       string
	}{
		"otherName SAN": {
			attributes: []asn1.RawValue{
				extensionRequestWithOtherName,
			},
			want: "is not a DNS name",
		},
		"challengePassword attribute": {
			attributes: []asn1.RawValue{
				extensionRequest,
				challengePassword,
			},
			want: "attribute 1.2.840.113549.1.9.7 is not supported",
		},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			csrPEM, metadata := testRawCSRPEM(
				t,
				definition,
				testCase.attributes,
			)
			if _, err := ParsePEM(csrPEM); err != nil {
				t.Fatalf("ParsePEM() error: %v", err)
			}
			if _, err := attachPEM(
				metadata,
				csrPEM,
			); err == nil ||
				!strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("attachPEM() error = %v", err)
			}
		})
	}
}

func TestIdentityAndDefinitionValidation(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	csrPEM := testCSRPEM(t, definition)
	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}
	identity := Identity{
		ID:                "csr-id",
		FingerprintSHA256: parsed.SHA256Fingerprint,
	}
	if err := ValidateIdentity(identity); err != nil {
		t.Fatalf("ValidateIdentity() error: %v", err)
	}
	if err := ValidateDefinition(definition); err != nil {
		t.Fatalf("ValidateDefinition() error: %v", err)
	}

	for _, invalid := range []Identity{
		{},
		{ID: "csr id", FingerprintSHA256: parsed.SHA256Fingerprint},
		{ID: "csr-id", FingerprintSHA256: "abc"},
		{
			ID:                "csr-id",
			FingerprintSHA256: strings.ToUpper(parsed.SHA256Fingerprint),
		},
	} {
		if err := ValidateIdentity(invalid); err == nil {
			t.Errorf("ValidateIdentity(%#v) returned no error", invalid)
		}
	}
}

type scriptedRequest struct {
	method     string
	path       string
	values     url.Values
	response   any
	statusCode int
	beforeSend func()
}

func newScriptedClient(
	t *testing.T,
	requests []scriptedRequest,
) *Client {
	t.Helper()

	var mu sync.Mutex
	requestIndex := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		mu.Lock()
		defer mu.Unlock()

		if requestIndex >= len(requests) {
			t.Errorf(
				"unexpected request %s %s",
				request.Method,
				request.URL.String(),
			)
			response.WriteHeader(http.StatusInternalServerError)

			return
		}
		expected := requests[requestIndex]
		requestIndex++

		if request.Method != expected.method {
			t.Errorf(
				"request %d method = %s, want %s",
				requestIndex,
				request.Method,
				expected.method,
			)
		}
		if request.URL.Path != expected.path {
			t.Errorf(
				"request %d path = %s, want %s",
				requestIndex,
				request.URL.Path,
				expected.path,
			)
		}
		if strings.Contains(request.URL.Path, "_key") &&
			request.URL.Path != "/execute/SSL/list_keys" {
			t.Errorf(
				"request %d called forbidden key operation %s",
				requestIndex,
				request.URL.Path,
			)
		}

		var actualValues url.Values
		if request.Method == http.MethodPost {
			if request.URL.RawQuery != "" {
				t.Errorf(
					"request %d query = %q, want empty",
					requestIndex,
					request.URL.RawQuery,
				)
			}
			if request.Header.Get("Content-Type") !=
				"application/x-www-form-urlencoded" {
				t.Errorf(
					"request %d Content-Type = %q",
					requestIndex,
					request.Header.Get("Content-Type"),
				)
			}
			if err := request.ParseForm(); err != nil {
				t.Errorf(
					"request %d ParseForm() error: %v",
					requestIndex,
					err,
				)
			}
			actualValues = request.Form
		} else {
			actualValues = request.URL.Query()
		}
		if expected.values == nil {
			expected.values = url.Values{}
		}
		if !reflect.DeepEqual(actualValues, expected.values) {
			t.Errorf(
				"request %d values = %v, want %v",
				requestIndex,
				actualValues,
				expected.values,
			)
		}

		statusCode := expected.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if expected.beforeSend != nil {
			expected.beforeSend()
		}
		response.WriteHeader(statusCode)
		if expected.response != nil {
			if err := json.NewEncoder(response).Encode(
				expected.response,
			); err != nil {
				t.Errorf(
					"request %d Encode() error: %v",
					requestIndex,
					err,
				)
			}
		}
	}))
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		defer mu.Unlock()
		if requestIndex != len(requests) {
			t.Errorf(
				"received %d requests, want %d",
				requestIndex,
				len(requests),
			)
		}
	})

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	client := NewClient(baseClient)
	client.generationRecoveryDelay = 2 * time.Second
	client.generationRecoveryLimit = time.Second

	return client
}

func successResponse(data any) map[string]any {
	return map[string]any{
		"status": 1,
		"data":   data,
	}
}

func testDefinition() Definition {
	return Definition{
		KeyID:                  "existing-key-id",
		FriendlyName:           "Terraform CSR",
		Domains:                []string{"example.test", "www.example.test"},
		CountryName:            "US",
		StateOrProvinceName:    "New York",
		LocalityName:           "New York",
		OrganizationName:       "Example Organization",
		OrganizationalUnitName: "Platform",
		EmailAddress:           "admin@example.test",
	}
}

func testCSRMetadata(definition Definition) CSR {
	domains := append([]string(nil), definition.Domains...)
	sort.Strings(domains)

	return CSR{
		ID:                     "csr-id",
		FriendlyName:           definition.FriendlyName,
		CommonName:             definition.Domains[0],
		CountryName:            definition.CountryName,
		StateOrProvinceName:    definition.StateOrProvinceName,
		LocalityName:           definition.LocalityName,
		OrganizationName:       definition.OrganizationName,
		OrganizationalUnitName: definition.OrganizationalUnitName,
		EmailAddress:           definition.EmailAddress,
		Created:                1700000000,
		Domains:                domains,
		KeyAlgorithm:           "id-ecPublicKey",
		ECDSACurveName:         "prime256v1",
		ECDSAPublic:            testECDSAPublicHex(),
	}
}

func inventoryItem(
	definition Definition,
	id any,
	created any,
) map[string]any {
	return map[string]any{
		"id":               id,
		"friendly_name":    definition.FriendlyName,
		"commonName":       definition.Domains[0],
		"created":          created,
		"domains":          append([]string(nil), definition.Domains...),
		"key_algorithm":    "id-ecPublicKey",
		"modulus":          nil,
		"ecdsa_curve_name": "prime256v1",
		"ecdsa_public":     testECDSAPublicHex(),
	}
}

func inventoryItemWithKey(
	definition Definition,
	key publicKeyMetadata,
) map[string]any {
	item := inventoryItem(definition, "csr-id", 1700000000)
	applyPublicKeyMetadata(item, key)

	return item
}

func showDetails(
	definition Definition,
	id string,
	created any,
) map[string]any {
	return map[string]any{
		"id":                     id,
		"friendly_name":          definition.FriendlyName,
		"commonName":             definition.Domains[0],
		"countryName":            definition.CountryName,
		"stateOrProvinceName":    definition.StateOrProvinceName,
		"localityName":           definition.LocalityName,
		"organizationName":       definition.OrganizationName,
		"organizationalUnitName": definition.OrganizationalUnitName,
		"emailAddress":           definition.EmailAddress,
		"created":                created,
		"domains": append(
			[]string(nil),
			definition.Domains...,
		),
		"key_algorithm":    "id-ecPublicKey",
		"modulus":          nil,
		"ecdsa_curve_name": "prime256v1",
		"ecdsa_public":     testECDSAPublicHex(),
	}
}

func showDetailsWithKey(
	definition Definition,
	id string,
	created any,
	key publicKeyMetadata,
) map[string]any {
	details := showDetails(definition, id, created)
	applyPublicKeyMetadata(details, key)

	return details
}

func showData(
	definition Definition,
	id string,
	created any,
	csrPEM string,
) map[string]any {
	return map[string]any{
		"csr": csrPEM,
		"details": showDetails(
			definition,
			id,
			created,
		),
	}
}

func showDataWithKey(
	definition Definition,
	csrPEM string,
	key publicKeyMetadata,
) map[string]any {
	return map[string]any{
		"csr": csrPEM,
		"details": showDetailsWithKey(
			definition,
			"csr-id",
			1700000000,
			key,
		),
	}
}

func generatedData(
	definition Definition,
	id string,
	created any,
	csrPEM string,
) map[string]any {
	data := inventoryItem(definition, id, created)
	data["text"] = csrPEM

	return data
}

func generatedDataWithKey(
	definition Definition,
	csrPEM string,
	key publicKeyMetadata,
) map[string]any {
	data := generatedData(
		definition,
		"csr-id",
		1700000000,
		csrPEM,
	)
	applyPublicKeyMetadata(data, key)

	return data
}

func publicKeyInventoryItem(key publicKeyMetadata) map[string]any {
	item := map[string]any{"id": key.ID}
	applyPublicKeyMetadata(item, key)

	return item
}

func applyPublicKeyMetadata(
	item map[string]any,
	key publicKeyMetadata,
) {
	item["key_algorithm"] = key.KeyAlgorithm
	item["modulus"] = nullableTestString(key.Modulus)
	item["ecdsa_curve_name"] = nullableTestString(key.ECDSACurveName)
	item["ecdsa_public"] = nullableTestString(key.ECDSAPublic)
}

func nullableTestString(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func generateValues(definition Definition) url.Values {
	return url.Values{
		"countryName": {definition.CountryName},
		"domains": {
			strings.Join(definition.Domains, ","),
		},
		"emailAddress": {definition.EmailAddress},
		"friendly_name": {
			definition.FriendlyName,
		},
		"key_id":           {definition.KeyID},
		"localityName":     {definition.LocalityName},
		"organizationName": {definition.OrganizationName},
		"organizationalUnitName": {
			definition.OrganizationalUnitName,
		},
		"stateOrProvinceName": {
			definition.StateOrProvinceName,
		},
	}
}

func testCSRPEM(t *testing.T, definition Definition) string {
	t.Helper()

	return testCSRPEMWithPrivateKey(
		t,
		definition,
		testECDSAPrivateKey(),
	)
}

func testRSACSRPEM(t *testing.T, definition Definition) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error: %v", err)
	}

	return testCSRPEMWithPrivateKey(t, definition, privateKey)
}

func testCSRPEMWithPrivateKey(
	t *testing.T,
	definition Definition,
	privateKey any,
) string {
	t.Helper()

	return testCSRPEMWithTemplateAndPrivateKey(
		t,
		testCSRTemplate(definition),
		privateKey,
	)
}

func testCSRTemplate(
	definition Definition,
) *x509.CertificateRequest {
	return &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:         definition.Domains[0],
			Country:            []string{definition.CountryName},
			Province:           []string{definition.StateOrProvinceName},
			Locality:           []string{definition.LocalityName},
			Organization:       []string{definition.OrganizationName},
			OrganizationalUnit: []string{definition.OrganizationalUnitName},
			ExtraNames: []pkix.AttributeTypeAndValue{{
				Type:  asn1.ObjectIdentifier(emailAddressOID),
				Value: definition.EmailAddress,
			}},
		},
		DNSNames: append([]string(nil), definition.Domains...),
	}
}

func testCSRPEMWithTemplateAndPrivateKey(
	t *testing.T,
	template *x509.CertificateRequest,
	privateKey any,
) string {
	t.Helper()

	requestDER, err := x509.CreateCertificateRequest(
		rand.Reader,
		template,
		privateKey,
	)
	if err != nil {
		t.Fatalf("CreateCertificateRequest() error: %v", err)
	}

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: requestDER,
	}))
}

func testRawCSRPEM(
	t *testing.T,
	definition Definition,
	attributes []asn1.RawValue,
) (string, CSR) {
	t.Helper()

	return testRawCSRPEMWithVersion(
		t,
		definition,
		0,
		attributes,
	)
}

func testRawCSRPEMWithVersion(
	t *testing.T,
	definition Definition,
	version int,
	attributes []asn1.RawValue,
) (string, CSR) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error: %v", err)
	}
	publicKeyDER, err := x509.MarshalPKIXPublicKey(
		&privateKey.PublicKey,
	)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey() error: %v", err)
	}
	publicKey := testRawValueFromDER(t, publicKeyDER)

	subjectDER, err := asn1.Marshal(
		testCSRTemplate(definition).Subject.ToRDNSequence(),
	)
	if err != nil {
		t.Fatalf("marshal CSR subject: %v", err)
	}
	subject := testRawValueFromDER(t, subjectDER)

	requestInfo := rawTBSCertificateRequest{
		Version:       version,
		Subject:       subject,
		PublicKey:     publicKey,
		RawAttributes: attributes,
	}
	requestInfoDER, err := asn1.Marshal(requestInfo)
	if err != nil {
		t.Fatalf("marshal certification request info: %v", err)
	}
	digest := sha256.Sum256(requestInfoDER)
	signature, err := rsa.SignPKCS1v15(
		rand.Reader,
		privateKey,
		crypto.SHA256,
		digest[:],
	)
	if err != nil {
		t.Fatalf("SignPKCS1v15() error: %v", err)
	}

	requestDER, err := asn1.Marshal(struct {
		RequestInfo        asn1.RawValue
		SignatureAlgorithm pkix.AlgorithmIdentifier
		SignatureValue     asn1.BitString
	}{
		RequestInfo: testRawValueFromDER(t, requestInfoDER),
		SignatureAlgorithm: pkix.AlgorithmIdentifier{
			Algorithm: asn1.ObjectIdentifier{
				1, 2, 840, 113549, 1, 1, 11,
			},
			Parameters: asn1.RawValue{Tag: asn1.TagNull},
		},
		SignatureValue: asn1.BitString{
			Bytes:     signature,
			BitLength: len(signature) * 8,
		},
	})
	if err != nil {
		t.Fatalf("marshal certificate request: %v", err)
	}

	metadata := testCSRMetadata(definition)
	metadata.KeyAlgorithm = "rsaEncryption"
	metadata.Modulus = hex.EncodeToString(privateKey.N.Bytes())
	metadata.ECDSACurveName = ""
	metadata.ECDSAPublic = ""

	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: requestDER,
	})), metadata
}

func testRawDNSNames(domains []string) []asn1.RawValue {
	names := make([]asn1.RawValue, 0, len(domains))
	for _, domain := range domains {
		names = append(names, asn1.RawValue{
			Class: asn1.ClassContextSpecific,
			Tag:   2,
			Bytes: []byte(domain),
		})
	}

	return names
}

func testRawExtensionRequestAttribute(
	t *testing.T,
	names []asn1.RawValue,
) asn1.RawValue {
	t.Helper()

	sanDER, err := asn1.Marshal(names)
	if err != nil {
		t.Fatalf("marshal SAN general names: %v", err)
	}
	extensionsDER, err := asn1.Marshal([]pkix.Extension{{
		Id:    asn1.ObjectIdentifier(subjectAltNameOID),
		Value: sanDER,
	}})
	if err != nil {
		t.Fatalf("marshal CSR extensions: %v", err)
	}

	return testRawPKCS10Attribute(
		t,
		asn1.ObjectIdentifier(extensionRequestOID),
		testRawValueFromDER(t, extensionsDER),
	)
}

func testRawPKCS10Attribute(
	t *testing.T,
	oid asn1.ObjectIdentifier,
	values ...asn1.RawValue,
) asn1.RawValue {
	t.Helper()

	attributeDER, err := asn1.Marshal(rawPKCS10Attribute{
		ID:     oid,
		Values: values,
	})
	if err != nil {
		t.Fatalf("marshal PKCS#10 attribute: %v", err)
	}

	return testRawValueFromDER(t, attributeDER)
}

func testRawValueFromDER(
	t *testing.T,
	value []byte,
) asn1.RawValue {
	t.Helper()

	var raw asn1.RawValue
	rest, err := asn1.Unmarshal(value, &raw)
	if err != nil {
		t.Fatalf("decode test DER value: %v", err)
	}
	if len(rest) != 0 {
		t.Fatal("test DER value contains trailing data")
	}

	return raw
}

func testASN1MarshalWithParams(
	t *testing.T,
	value any,
	params string,
) []byte {
	t.Helper()

	encoded, err := asn1.MarshalWithParams(value, params)
	if err != nil {
		t.Fatalf("MarshalWithParams() error: %v", err)
	}

	return encoded
}

func testPublicKeyFromCSR(
	t *testing.T,
	id string,
	csrPEM string,
) publicKeyMetadata {
	t.Helper()

	parsed, err := ParsePEM(csrPEM)
	if err != nil {
		t.Fatalf("ParsePEM() error: %v", err)
	}

	key := publicKeyMetadata{ID: id}
	switch publicKey := parsed.Request.PublicKey.(type) {
	case *rsa.PublicKey:
		key.KeyAlgorithm = "rsaEncryption"
		key.Modulus = hex.EncodeToString(publicKey.N.Bytes())
	case *ecdsa.PublicKey:
		key.KeyAlgorithm = "id-ecPublicKey"
		switch publicKey.Curve.Params().Name {
		case elliptic.P256().Params().Name:
			key.ECDSACurveName = "prime256v1"
		case elliptic.P384().Params().Name:
			key.ECDSACurveName = "secp384r1"
		case elliptic.P521().Params().Name:
			key.ECDSACurveName = "secp521r1"
		default:
			t.Fatalf(
				"unsupported test ECDSA curve %q",
				publicKey.Curve.Params().Name,
			)
		}
		key.ECDSAPublic = testCompressedECDSAPublic(publicKey)
	default:
		t.Fatalf(
			"unsupported test public key type %T",
			parsed.Request.PublicKey,
		)
	}

	return key
}

func testECDSAPrivateKey() *ecdsa.PrivateKey {
	return testECDSAPrivateKeyFromScalar(1)
}

func testECDSAPrivateKeyFromScalar(
	scalar int64,
) *ecdsa.PrivateKey {
	if scalar < 1 || scalar > 255 {
		panic("test ECDSA scalar must be between 1 and 255")
	}

	rawPrivateKey := make([]byte, 32)
	rawPrivateKey[len(rawPrivateKey)-1] = byte(scalar)
	privateKey, err := ecdsa.ParseRawPrivateKey(
		elliptic.P256(),
		rawPrivateKey,
	)
	if err != nil {
		panic(fmt.Sprintf("parse test ECDSA private key: %v", err))
	}

	return privateKey
}

func testECDSAPublicHex() string {
	key := testECDSAPrivateKey()
	publicKey, ok := key.Public().(*ecdsa.PublicKey)
	if !ok {
		panic(fmt.Sprintf(
			"test ECDSA public key type = %T",
			key.Public(),
		))
	}

	return testCompressedECDSAPublic(publicKey)
}

func testCompressedECDSAPublic(publicKey *ecdsa.PublicKey) string {
	uncompressed, err := publicKey.Bytes()
	if err != nil {
		panic(fmt.Sprintf("encode test ECDSA public key: %v", err))
	}
	if len(uncompressed) < 3 || uncompressed[0] != 0x04 ||
		(len(uncompressed)-1)%2 != 0 {
		panic("unexpected uncompressed test ECDSA public key")
	}

	coordinateLength := (len(uncompressed) - 1) / 2
	compressed := make([]byte, coordinateLength+1)
	compressed[0] = 0x02 | (uncompressed[len(uncompressed)-1] & 0x01)
	copy(compressed[1:], uncompressed[1:coordinateLength+1])

	return hex.EncodeToString(compressed)
}
