package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccountCapabilitiesDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewAccountCapabilitiesDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{
		"account",
		"cpanel_version",
		"username",
		"home_directory",
		"primary_domain",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(datasourceschema.StringAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed string", attributeName)
		}
	}

	features, ok := response.Schema.Attributes["features"].(datasourceschema.MapAttribute)
	if !ok || !features.Computed || features.Sensitive {
		t.Fatal("features must be a non-sensitive computed map")
	}
}

func TestAccAccountCapabilitiesDataSource(t *testing.T) {
	const dataSourceName = "data.cpanel_account_capabilities.current"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: providerConfig + `
data "cpanel_account_capabilities" "current" {}
`,
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"account",
						"account",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"username",
						os.Getenv("CPANEL_USERNAME"),
					),
					testresource.TestCheckResourceAttrSet(
						dataSourceName,
						"cpanel_version",
					),
					testresource.TestCheckResourceAttrSet(
						dataSourceName,
						"home_directory",
					),
					testresource.TestCheckResourceAttrSet(
						dataSourceName,
						"primary_domain",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"features.apitokens",
						"true",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"features.passengerapps",
						"true",
					),
					testresource.TestCheckTypeSetElemAttr(
						dataSourceName,
						"enabled_features.*",
						"passengerapps",
					),
					testresource.TestCheckResourceAttrSet(
						dataSourceName,
						"limits.passenger_apps",
					),
				),
			},
		},
	})
}
