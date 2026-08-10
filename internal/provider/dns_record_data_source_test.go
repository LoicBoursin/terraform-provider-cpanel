package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDNSRecordDataSource(t *testing.T) {
	name := testAccDNSRecordName(t, "datasource")
	zone := testAccMainDomain(t)
	data := []string{testAccDNSRecordData("datasource")}
	testAccRegisterDNSRecord(t, zone, name, "TXT", 300, data)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDNSRecordsDestroyed(zone, name),
		Steps: []resource.TestStep{
			{
				Config: testAccDNSRecordDataSourceConfig(zone, name, data[0]),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_dns_record.test",
						"zone",
						zone,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dns_record.test",
						"name",
						name,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dns_record.test",
						"type",
						"TXT",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dns_record.test",
						"ttl",
						"300",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_dns_record.test",
						"data.0",
						data[0],
					),
					testAccCheckDNSRecordExists(
						zone,
						name,
						300,
						data,
					),
				),
			},
		},
	})
}

func testAccDNSRecordDataSourceConfig(zone, name, data string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_dns_record" "fixture" {
  zone = %q
  name = %q
  type = "TXT"
  ttl  = 300
  data = [%q]
}

data "cpanel_dns_record" "test" {
  zone       = cpanel_dns_record.fixture.zone
  line_index = cpanel_dns_record.fixture.line_index
}
`, zone, name, data)
}
