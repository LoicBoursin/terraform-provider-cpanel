package provider

import (
	"strings"
	"testing"
)

func TestValidateDNSRecordTypes(t *testing.T) {
	t.Parallel()

	testCases := map[string][]string{
		"A":     {"192.0.2.10"},
		"AAAA":  {"2001:db8::10"},
		"CAA":   {"0", "issue", "letsencrypt.org"},
		"CNAME": {"target.example.test."},
		"MX":    {"10", "mail.example.test."},
		"SRV":   {"0", "5", "443", "service.example.test."},
		"TXT":   {"terraform-provider-cpanel"},
	}

	for recordType, data := range testCases {
		recordType := recordType
		data := data
		t.Run(recordType, func(t *testing.T) {
			t.Parallel()

			if err := validateDNSRecord(
				"example.test",
				"tfcpaneldns",
				recordType,
				300,
				data,
			); err != nil {
				t.Fatalf("validateDNSRecord() error: %v", err)
			}
		})
	}
}

func TestValidateDNSRecordRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		zone       string
		recordName string
		recordType string
		ttl        int64
		data       []string
	}{
		{
			name:       "fully qualified name",
			zone:       "example.test",
			recordName: "www.example.test",
			recordType: "A",
			ttl:        300,
			data:       []string{"192.0.2.10"},
		},
		{
			name:       "lowercase type",
			zone:       "example.test",
			recordName: "www",
			recordType: "txt",
			ttl:        300,
			data:       []string{"value"},
		},
		{
			name:       "invalid IPv4",
			zone:       "example.test",
			recordName: "www",
			recordType: "A",
			ttl:        300,
			data:       []string{"2001:db8::10"},
		},
		{
			name:       "missing MX exchange",
			zone:       "example.test",
			recordName: "@",
			recordType: "MX",
			ttl:        300,
			data:       []string{"10"},
		},
		{
			name:       "invalid TTL",
			zone:       "example.test",
			recordName: "www",
			recordType: "TXT",
			ttl:        0,
			data:       []string{"value"},
		},
		{
			name:       "oversized TXT chunk",
			zone:       "example.test",
			recordName: "www",
			recordType: "TXT",
			ttl:        300,
			data:       []string{strings.Repeat("x", 256)},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			if err := validateDNSRecord(
				testCase.zone,
				testCase.recordName,
				testCase.recordType,
				testCase.ttl,
				testCase.data,
			); err == nil {
				t.Fatal("validateDNSRecord() returned no error")
			}
		})
	}
}
