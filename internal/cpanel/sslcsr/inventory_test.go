package sslcsr

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestClientListsSafeCSRMetadataWithoutShowingPEM(t *testing.T) {
	t.Parallel()

	ecdsaDefinition := testDefinition()
	ecdsaDefinition.FriendlyName = "ECDSA CSR"
	ecdsaItem := inventoryItem(
		ecdsaDefinition,
		"csr-z",
		1700000001,
	)
	rsaDefinition := testDefinition()
	rsaDefinition.FriendlyName = "RSA CSR"
	rsaPEM := testRSACSRPEM(t, rsaDefinition)
	rsaKey := testPublicKeyFromCSR(t, "rsa-key", rsaPEM)
	rsaItem := inventoryItemWithKey(rsaDefinition, rsaKey)
	rsaItem["id"] = "csr-a"
	rsaItem["created"] = "1700000000"

	client := newScriptedClient(t, []scriptedRequest{{
		method: http.MethodGet,
		path:   "/execute/SSL/list_csrs",
		response: successResponse([]map[string]any{
			ecdsaItem,
			rsaItem,
		}),
	}})
	csrs, err := client.ListMetadata(t.Context())
	if err != nil {
		t.Fatalf("ListMetadata() error: %v", err)
	}
	if len(csrs) != 2 || csrs[0].ID != "csr-a" || csrs[1].ID != "csr-z" {
		t.Fatalf("CSRs = %#v", csrs)
	}
	rsa := csrs[0]
	if rsa.FriendlyName != "RSA CSR" ||
		rsa.CommonName != rsaDefinition.Domains[0] ||
		!reflect.DeepEqual(
			rsa.Domains,
			[]string{"example.test", "www.example.test"},
		) ||
		rsa.Created != 1700000000 ||
		rsa.KeyAlgorithm != "rsaEncryption" ||
		rsa.ModulusLength == nil ||
		*rsa.ModulusLength != 2048 ||
		rsa.ECDSACurveName != "" {
		t.Fatalf("RSA CSR = %#v", rsa)
	}
	ecdsa := csrs[1]
	if ecdsa.KeyAlgorithm != "id-ecPublicKey" ||
		ecdsa.ModulusLength != nil ||
		ecdsa.ECDSACurveName != "prime256v1" {
		t.Fatalf("ECDSA CSR = %#v", ecdsa)
	}
}

func TestClientListsSafeSSLKeyMetadata(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	rsaPEM := testRSACSRPEM(t, definition)
	rsaKey := testPublicKeyFromCSR(t, "key-z", rsaPEM)
	rsaItem := publicKeyInventoryItem(rsaKey)
	rsaItem["friendly_name"] = "RSA key"
	rsaItem["created"] = "1700000000"
	rsaItem["modulus_length"] = "2048"

	ecdsaPEM := testCSRPEM(t, definition)
	ecdsaKey := testPublicKeyFromCSR(t, "key-a", ecdsaPEM)
	ecdsaItem := publicKeyInventoryItem(ecdsaKey)
	ecdsaItem["friendly_name"] = "ECDSA key"
	ecdsaItem["created"] = 1700000001
	ecdsaItem["modulus_length"] = nil

	client := newScriptedClient(t, []scriptedRequest{{
		method: http.MethodGet,
		path:   "/execute/SSL/list_keys",
		response: successResponse([]map[string]any{
			rsaItem,
			ecdsaItem,
		}),
	}})
	keys, err := client.ListKeys(t.Context())
	if err != nil {
		t.Fatalf("ListKeys() error: %v", err)
	}
	if len(keys) != 2 || keys[0].ID != "key-a" || keys[1].ID != "key-z" {
		t.Fatalf("keys = %#v", keys)
	}
	if keys[0].FriendlyName != "ECDSA key" ||
		keys[0].Created != 1700000001 ||
		keys[0].KeyAlgorithm != "id-ecPublicKey" ||
		keys[0].ModulusLength != nil ||
		keys[0].ECDSACurveName != "prime256v1" {
		t.Fatalf("ECDSA key = %#v", keys[0])
	}
	if keys[1].FriendlyName != "RSA key" ||
		keys[1].Created != 1700000000 ||
		keys[1].KeyAlgorithm != "rsaEncryption" ||
		keys[1].ModulusLength == nil ||
		*keys[1].ModulusLength != 2048 ||
		keys[1].ECDSACurveName != "" {
		t.Fatalf("RSA key = %#v", keys[1])
	}
}

func TestClientRejectsUnsafeSSLMetadataInventories(t *testing.T) {
	t.Parallel()

	definition := testDefinition()
	ecdsaCSR := inventoryItem(definition, "csr-id", 1700000000)
	rsaPEM := testRSACSRPEM(t, definition)
	rsaKey := testPublicKeyFromCSR(t, "key-id", rsaPEM)
	validKey := publicKeyInventoryItem(rsaKey)
	validKey["friendly_name"] = "RSA key"
	validKey["created"] = 1700000000
	validKey["modulus_length"] = 2048

	testCases := []struct {
		name      string
		path      string
		inventory []map[string]any
		run       func(*Client) error
	}{
		{
			name: "CSR missing public point",
			path: "/execute/SSL/list_csrs",
			inventory: []map[string]any{func() map[string]any {
				item := cloneStringAnyMap(ecdsaCSR)
				item["ecdsa_public"] = nil

				return item
			}()},
			run: func(client *Client) error {
				_, err := client.ListMetadata(t.Context())

				return err
			},
		},
		{
			name:      "duplicate CSR id",
			path:      "/execute/SSL/list_csrs",
			inventory: []map[string]any{ecdsaCSR, ecdsaCSR},
			run: func(client *Client) error {
				_, err := client.ListMetadata(t.Context())

				return err
			},
		},
		{
			name: "key missing friendly name",
			path: "/execute/SSL/list_keys",
			inventory: []map[string]any{func() map[string]any {
				item := cloneStringAnyMap(validKey)
				delete(item, "friendly_name")

				return item
			}()},
			run: func(client *Client) error {
				_, err := client.ListKeys(t.Context())

				return err
			},
		},
		{
			name: "key modulus length mismatch",
			path: "/execute/SSL/list_keys",
			inventory: []map[string]any{func() map[string]any {
				item := cloneStringAnyMap(validKey)
				item["modulus_length"] = 1024

				return item
			}()},
			run: func(client *Client) error {
				_, err := client.ListKeys(t.Context())

				return err
			},
		},
		{
			name:      "duplicate key id",
			path:      "/execute/SSL/list_keys",
			inventory: []map[string]any{validKey, validKey},
			run: func(client *Client) error {
				_, err := client.ListKeys(t.Context())

				return err
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := newScriptedClient(t, []scriptedRequest{{
				method:   http.MethodGet,
				path:     testCase.path,
				response: successResponse(testCase.inventory),
			}})
			err := testCase.run(client)
			if err == nil {
				t.Fatal("inventory call returned no error")
			}
			if strings.Contains(err.Error(), "show_csr") {
				t.Fatalf("inventory error unexpectedly references PEM: %v", err)
			}
		})
	}
}

func cloneStringAnyMap(value map[string]any) map[string]any {
	clone := make(map[string]any, len(value))
	for key, item := range value {
		clone[key] = item
	}

	return clone
}
