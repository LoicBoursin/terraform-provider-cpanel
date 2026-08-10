package gpg

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseInventoryAcceptsFlexibleIntegersAndSorts(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`[
		{
			"algorithm":"RSA",
			"bits":"4096",
			"created":"1700000002",
			"expires":"",
			"id":"FFFFFFFFFFFFFFFF",
			"type":"pub",
			"user_id":"Second"
		},
		{
			"algorithm":"RSA",
			"bits":2048,
			"created":1700000001,
			"expires":1800000000,
			"id":"0000000000000001",
			"type":"pub",
			"user_id":"First"
		}
	]`)

	values, err := parseInventory(raw, "pub", 16)
	if err != nil {
		t.Fatalf("parseInventory() error: %v", err)
	}
	if len(values) != 2 {
		t.Fatalf("len(values) = %d, want 2", len(values))
	}
	if values[0].ID != "0000000000000001" {
		t.Fatalf("first ID = %q", values[0].ID)
	}
	if values[0].Expires == nil || *values[0].Expires != 1800000000 {
		t.Fatalf("first Expires = %#v", values[0].Expires)
	}
	if values[1].Expires != nil {
		t.Fatalf("second Expires = %#v, want nil", values[1].Expires)
	}
}

func TestParseInventoryRejectsMalformedShape(t *testing.T) {
	t.Parallel()

	valid := `{
		"algorithm":"RSA",
		"bits":"2048",
		"created":"1700000000",
		"expires":"",
		"id":"0123456789ABCDEF",
		"type":"pub",
		"user_id":"Test"
	}`
	testCases := map[string]string{
		"null":            `null`,
		"object":          valid,
		"unknown field":   `[{"algorithm":"RSA","bits":"2048","created":"1700000000","expires":"","id":"0123456789ABCDEF","type":"pub","user_id":"Test","extra":true}]`,
		"missing field":   `[{"algorithm":"RSA","bits":"2048","created":"1700000000","expires":"","id":"0123456789ABCDEF","type":"pub"}]`,
		"wrong type":      `[` + strings.Replace(valid, `"type":"pub"`, `"type":"sec"`, 1) + `]`,
		"duplicate ID":    `[` + valid + `,` + valid + `]`,
		"negative bits":   `[` + strings.Replace(valid, `"bits":"2048"`, `"bits":"-1"`, 1) + `]`,
		"bad created":     `[` + strings.Replace(valid, `"created":"1700000000"`, `"created":"now"`, 1) + `]`,
		"lowercase input": `[` + strings.Replace(valid, `"0123456789ABCDEF"`, `"0123456789abcdef"`, 1) + `]`,
	}
	for name, raw := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			values, err := parseInventory(
				json.RawMessage(raw),
				"pub",
				16,
			)
			if name == "lowercase input" {
				if err != nil {
					t.Fatalf("parseInventory() lowercase error: %v", err)
				}
				if values[0].ID != "0123456789ABCDEF" {
					t.Fatalf("normalized ID = %q", values[0].ID)
				}
				return
			}
			if err == nil {
				t.Fatalf("parseInventory() accepted %s", name)
			}
		})
	}
}

func TestParseSecretInventoryAcceptsShortID(t *testing.T) {
	t.Parallel()

	values, err := parseInventoryFlexibleSecretIDs(json.RawMessage(`[
		{
			"algorithm":"RSA",
			"bits":"2048",
			"created":"1700000000",
			"expires":"",
			"id":"89abcdef",
			"type":"sec",
			"user_id":"Test"
		}
	]`))
	if err != nil {
		t.Fatalf("parseInventoryFlexibleSecretIDs() error: %v", err)
	}
	if len(values) != 1 || values[0].ID != "89ABCDEF" {
		t.Fatalf("secret inventory = %#v", values)
	}
}
