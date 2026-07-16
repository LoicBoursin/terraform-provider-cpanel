package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestResourceUsageDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewResourceUsageDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	account, ok := response.Schema.Attributes["account"].(datasourceschema.StringAttribute)
	if !ok || !account.Computed || account.Sensitive {
		t.Fatal("account must be a non-sensitive computed string")
	}
	metrics, ok := response.Schema.Attributes["metrics"].(datasourceschema.ListNestedAttribute)
	if !ok || !metrics.Computed || metrics.Sensitive {
		t.Fatal("metrics must be a non-sensitive computed nested list")
	}
	for _, attributeName := range []string{
		"id",
		"usage",
		"maximum",
		"formatter",
	} {
		attribute, ok := metrics.NestedObject.Attributes[attributeName].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed || attribute.Sensitive {
			t.Fatalf("%s must be a non-sensitive computed string", attributeName)
		}
	}
}

func TestAccResourceUsageDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_resource_usage.current"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: providerConfig + `
data "cpanel_resource_usage" "current" {}
`,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"account",
						"account",
					),
					testresource.TestCheckResourceAttrSet(
						dataSourceName,
						"metrics.#",
					),
					testCheckResourceUsageMetric(
						dataSourceName,
						"disk_usage",
					),
					testCheckResourceUsageMetric(
						dataSourceName,
						"email_accounts",
					),
				),
			},
		},
	})
}

func testCheckResourceUsageMetric(
	dataSourceName string,
	metricID string,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[dataSourceName]
		if !ok {
			return fmt.Errorf("resource state %s not found", dataSourceName)
		}

		count := resourceState.Primary.Attributes["metrics.#"]
		for index := 0; ; index++ {
			prefix := fmt.Sprintf("metrics.%d.", index)
			id, exists := resourceState.Primary.Attributes[prefix+"id"]
			if !exists {
				break
			}
			if id != metricID {
				continue
			}
			if resourceState.Primary.Attributes[prefix+"usage"] == "" {
				return fmt.Errorf("%s usage is empty", metricID)
			}
			return nil
		}

		return fmt.Errorf(
			"resource usage metric %q not found in %s entries",
			metricID,
			count,
		)
	}
}
