package provider

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

func TestMySQLRestrictionsDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewMySQLRestrictionsDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	if len(response.Schema.Attributes) != 3 {
		t.Fatalf(
			"top-level attribute count = %d, want 3",
			len(response.Schema.Attributes),
		)
	}
	prefix, ok := response.Schema.Attributes["prefix"].(datasourceschema.StringAttribute)
	if !ok || !prefix.Computed || prefix.Optional || prefix.Required {
		t.Fatal("prefix must be a computed string")
	}
	maxDatabaseNameLength, ok := response.Schema.Attributes["max_database_name_length"].(datasourceschema.Int64Attribute)
	if !ok || !maxDatabaseNameLength.Computed ||
		maxDatabaseNameLength.Optional || maxDatabaseNameLength.Required {
		t.Fatal("max_database_name_length must be a computed integer")
	}
	maxUsernameLength, ok := response.Schema.Attributes["max_username_length"].(datasourceschema.Int64Attribute)
	if !ok || !maxUsernameLength.Computed ||
		maxUsernameLength.Optional || maxUsernameLength.Required {
		t.Fatal("max_username_length must be a computed integer")
	}
}

func TestAccMySQLRestrictionsDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadMySQLRestrictions(t)
	t.Cleanup(func() {
		testAccRequireMySQLRestrictions(t, baseline)
	})

	const (
		dataSourceName = "data.cpanel_mysql_restrictions.account"
		config         = providerConfig + `
data "cpanel_mysql_restrictions" "account" {}
`
	)
	checks := testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"prefix",
			baseline.Prefix,
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"max_database_name_length",
			strconv.Itoa(baseline.MaxDatabaseNameLength),
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"max_username_length",
			strconv.Itoa(baseline.MaxUsernameLength),
		),
	)

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireMySQLRestrictions(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check:  checks,
			},
			{
				Config: config,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccReadMySQLRestrictions(
	t *testing.T,
) mysql.Restrictions {
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
	restrictions, err := mysql.NewClient(client).GetRestrictions(ctx)
	if err != nil {
		t.Fatalf("read cPanel MySQL restrictions: %v", err)
	}

	return *restrictions
}

func testAccRequireMySQLRestrictions(
	t *testing.T,
	expected mysql.Restrictions,
) {
	t.Helper()

	actual := testAccReadMySQLRestrictions(t)
	if actual != expected {
		t.Fatalf(
			"cPanel MySQL restrictions changed: got %#v, expected %#v",
			actual,
			expected,
		)
	}
}
