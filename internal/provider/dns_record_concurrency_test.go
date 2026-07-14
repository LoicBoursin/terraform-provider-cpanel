package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSRecordConcurrentResources(t *testing.T) {
	firstName := testAccDNSRecordName(t, "concurrentfirst")
	secondName := testAccDNSRecordName(t, "concurrentsecond")
	zone := testAccMainDomain(t)
	firstData := []string{testAccDNSRecordData("concurrent-first")}
	secondData := []string{testAccDNSRecordData("concurrent-second")}
	testAccRegisterDNSRecord(t, zone, firstName, "TXT", 300, firstData)
	testAccRegisterDNSRecord(t, zone, secondName, "TXT", 300, secondData)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDNSRecordsDestroyed(
			zone,
			firstName,
			secondName,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccDNSRecordConcurrentConfig(
					zone,
					firstName,
					firstData[0],
					secondName,
					secondData[0],
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDNSRecordExists(
						zone,
						firstName,
						300,
						firstData,
					),
					testAccCheckDNSRecordExists(
						zone,
						secondName,
						300,
						secondData,
					),
				),
			},
		},
	})
}

func testAccDNSRecordConcurrentConfig(
	zone string,
	firstName string,
	firstData string,
	secondName string,
	secondData string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_dns_record" "first" {
  zone = %q
  name = %q
  type = "TXT"
  ttl  = 300
  data = [%q]
}

resource "cpanel_dns_record" "second" {
  zone = %q
  name = %q
  type = "TXT"
  ttl  = 300
  data = [%q]
}
`, zone, firstName, firstData, zone, secondName, secondData)
}
