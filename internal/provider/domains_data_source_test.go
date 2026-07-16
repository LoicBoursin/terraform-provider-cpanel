package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

func TestDomainsDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewDomainsDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	domains, ok := response.Schema.Attributes["domains"].(datasourceschema.ListNestedAttribute)
	if !ok || !domains.Computed {
		t.Fatal("domains must be a computed nested list")
	}
	if len(domains.NestedObject.Attributes) != 2 {
		t.Fatalf(
			"domain nested attribute count = %d",
			len(domains.NestedObject.Attributes),
		)
	}
}

func TestAccDomainsDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadDomains(t)
	t.Cleanup(func() {
		testAccRequireDomains(t, baseline)
	})

	const dataSourceName = "data.cpanel_domains.all"
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(
			dataSourceName,
			"domains.#",
			strconv.Itoa(len(baseline)),
		),
	}
	for index, domain := range baseline {
		checks = append(
			checks,
			resource.TestCheckResourceAttr(
				dataSourceName,
				fmt.Sprintf("domains.%d.name", index),
				domain.Name,
			),
			resource.TestCheckResourceAttr(
				dataSourceName,
				fmt.Sprintf("domains.%d.type", index),
				string(domain.Type),
			),
		)
	}

	const config = providerConfig + `
data "cpanel_domains" "all" {}
`
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireDomains(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.ComposeAggregateTestCheckFunc(checks...),
			},
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccReadDomains(
	t *testing.T,
) []cpaneldomain.InventoryDomain {
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
	domains, err := cpaneldomain.NewClient(client).ListDomains(ctx)
	if err != nil {
		t.Fatalf("read cPanel domain inventory: %v", err)
	}

	return domains
}

func testAccRequireDomains(
	t *testing.T,
	expected []cpaneldomain.InventoryDomain,
) {
	t.Helper()

	actual := testAccReadDomains(t)
	if len(actual) != len(expected) {
		t.Fatalf(
			"cPanel domain count changed: got %d, expected %d",
			len(actual),
			len(expected),
		)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf(
				"cPanel domain inventory changed at index %d",
				index,
			)
		}
	}
}
