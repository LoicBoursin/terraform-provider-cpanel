package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestValidateDNSRecordManagedStateRequiresCompleteDefinition(t *testing.T) {
	t.Parallel()

	complete := DNSRecordModel{
		Zone:       types.StringValue("example.test"),
		Name:       types.StringValue("www"),
		RecordType: types.StringValue("A"),
		TTL:        types.Int64Value(300),
		Data: types.ListValueMust(
			types.StringType,
			[]attr.Value{types.StringValue("192.0.2.10")},
		),
		LineIndex: types.Int64Value(11),
	}
	if err := validateDNSRecordManagedState(complete); err != nil {
		t.Fatalf("validateDNSRecordManagedState() error: %v", err)
	}

	incomplete := complete
	incomplete.Name = types.StringNull()
	if err := validateDNSRecordManagedState(incomplete); err == nil {
		t.Fatal("validateDNSRecordManagedState() returned no error")
	}
}

func TestAccDNSRecordResource(t *testing.T) {
	const resourceName = "cpanel_dns_record.test"

	firstName := testAccDNSRecordName(t, "resource")
	secondName := testAccDNSRecordName(t, "replacement")
	zone := testAccMainDomain(t)
	firstData := []string{testAccDNSRecordData("resource")}
	updatedData := []string{testAccDNSRecordData("updated")}
	secondData := []string{testAccDNSRecordData("replacement")}
	driftData := []string{testAccDNSRecordData("drift")}
	testAccRegisterDNSRecord(t, zone, firstName, "TXT", 300, firstData)
	testAccRegisterDNSRecord(t, zone, firstName, "TXT", 600, updatedData)
	testAccRegisterDNSRecord(t, zone, secondName, "TXT", 900, secondData)
	testAccRegisterDNSRecord(t, zone, secondName, "TXT", 1200, driftData)

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
				Config: testAccDNSRecordResourceConfig(
					zone,
					firstName,
					300,
					firstData[0],
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "zone", zone),
					resource.TestCheckResourceAttr(resourceName, "name", firstName),
					resource.TestCheckResourceAttr(resourceName, "type", "TXT"),
					resource.TestCheckResourceAttr(resourceName, "ttl", "300"),
					resource.TestCheckResourceAttr(resourceName, "data.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "data.0", firstData[0]),
					resource.TestCheckResourceAttrSet(resourceName, "line_index"),
					testAccCheckDNSRecordExists(
						zone,
						firstName,
						300,
						firstData,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateIdFunc:                    testAccDNSRecordImportStateID(resourceName, zone),
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "line_index",
			},
			{
				Config: testAccDNSRecordResourceConfig(
					zone,
					firstName,
					600,
					updatedData[0],
				),
				Check: testAccCheckDNSRecordExists(
					zone,
					firstName,
					600,
					updatedData,
				),
			},
			{
				Config: testAccDNSRecordResourceConfig(
					zone,
					secondName,
					900,
					secondData[0],
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckDNSRecordExists(
						zone,
						secondName,
						900,
						secondData,
					),
					testAccCheckDNSRecordsDestroyed(zone, firstName),
				),
			},
			{
				PreConfig: func() {
					testAccUpdateDNSRecord(
						t,
						zone,
						secondName,
						1200,
						driftData,
					)
				},
				Config:             testAccDNSRecordResourceConfig(zone, secondName, 900, secondData[0]),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDNSRecordResourceConfig(
					zone,
					secondName,
					900,
					secondData[0],
				),
				Check: testAccCheckDNSRecordExists(
					zone,
					secondName,
					900,
					secondData,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteDNSRecord(t, zone, secondName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDNSRecordResourceConfig(
					zone,
					secondName,
					900,
					secondData[0],
				),
				Check: testAccCheckDNSRecordExists(
					zone,
					secondName,
					900,
					secondData,
				),
			},
		},
	})
}

func testAccDNSRecordResourceConfig(
	zone string,
	name string,
	ttl int64,
	data string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_dns_record" "test" {
  zone = %q
  name = %q
  type = "TXT"
  ttl  = %d
  data = [%q]
}
`, zone, name, ttl, data)
}
