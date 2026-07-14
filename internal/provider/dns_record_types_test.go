package provider

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSRecordSupportedTypes(t *testing.T) {
	name := testAccDNSRecordName(t, "types")
	zone := testAccMainDomain(t)

	testCases := []struct {
		recordType string
		data       []string
	}{
		{recordType: "A", data: []string{"192.0.2.10"}},
		{recordType: "AAAA", data: []string{"2001:db8::10"}},
		{recordType: "CAA", data: []string{"0", "issue", "letsencrypt.org"}},
		{recordType: "CNAME", data: []string{zone + "."}},
		{recordType: "MX", data: []string{"10", "mail." + zone + "."}},
		{
			recordType: "SRV",
			data:       []string{"0", "5", "443", "service." + zone + "."},
		},
		{recordType: "TXT", data: []string{"terraform-provider-cpanel-types"}},
	}

	steps := make([]resource.TestStep, 0, len(testCases))
	for _, testCase := range testCases {
		testCase := testCase
		testAccRegisterDNSRecord(
			t,
			zone,
			name,
			testCase.recordType,
			300,
			testCase.data,
		)
		steps = append(steps, resource.TestStep{
			Config: testAccDNSRecordTypeConfig(
				t,
				zone,
				name,
				testCase.recordType,
				testCase.data,
			),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(
					"cpanel_dns_record.test",
					"type",
					testCase.recordType,
				),
				testAccCheckTypedDNSRecordExists(
					zone,
					name,
					testCase.recordType,
					300,
					testCase.data,
				),
			),
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSRecordsDestroyed(zone, name),
		Steps:                    steps,
	})
}

func testAccDNSRecordTypeConfig(
	t *testing.T,
	zone string,
	name string,
	recordType string,
	data []string,
) string {
	t.Helper()

	encodedData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("encode DNS record test data: %v", err)
	}

	return providerConfig + fmt.Sprintf(`
resource "cpanel_dns_record" "test" {
  zone = %q
  name = %q
  type = %q
  ttl  = 300
  data = %s
}
`, zone, name, recordType, encodedData)
}
