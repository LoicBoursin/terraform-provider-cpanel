package provider

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

func TestMySQLDatabasesDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewMySQLDatabasesDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	if len(response.Schema.Attributes) != 1 {
		t.Fatalf(
			"top-level attribute count = %d, want 1",
			len(response.Schema.Attributes),
		)
	}
	databases, ok := response.Schema.Attributes["databases"].(datasourceschema.ListAttribute)
	if !ok || !databases.Computed || databases.Optional || databases.Required {
		t.Fatal("databases must be a computed string list")
	}
	if databases.ElementType != types.StringType {
		t.Fatalf(
			"databases element type = %T, want string",
			databases.ElementType,
		)
	}
}

func TestAccMySQLDatabasesDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	databaseName := testAccMySQLName("idsd")
	userName := testAccMySQLName("idsdu")
	baseline := testAccReadMySQLDatabaseNames(t)
	t.Cleanup(func() {
		testAccRequireMySQLDatabaseNames(t, baseline)
	})
	expected := append(slices.Clone(baseline), databaseName)
	slices.Sort(expected)

	const dataSourceName = "data.cpanel_mysql_databases.all"
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"databases.#",
			strconv.Itoa(len(expected)),
		),
	}
	for index, database := range expected {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				dataSourceName,
				fmt.Sprintf("databases.%d", index),
				database,
			),
		)
	}

	config := testAccMySQLDatabasesDataSourceConfig(
		databaseName,
		userName,
	)
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireMySQLDatabaseNames(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeAggregateTestCheckFunc(
			testAccCheckMySQLDatabasesDestroyed(databaseName),
			testAccCheckMySQLUsersDestroyed(userName),
		),
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					checks...,
				),
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

func testAccMySQLDatabasesDataSourceConfig(
	databaseName string,
	userName string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "inventory_fixture" {
  name             = %q
  password         = "W7!mysqlInventoryDatabase-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_mysql_database" "inventory_fixture" {
  name              = %q
  users             = [cpanel_mysql_user.inventory_fixture.name]
  delete_on_destroy = true
}

data "cpanel_mysql_databases" "all" {
  depends_on = [cpanel_mysql_database.inventory_fixture]
}
`, userName, databaseName)
}

func testAccReadMySQLDatabaseNames(t *testing.T) []string {
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
	databases, err := mysql.NewClient(client).ListDatabaseNames(ctx)
	if err != nil {
		t.Fatalf("read cPanel MySQL database inventory: %v", err)
	}

	return databases
}

func testAccRequireMySQLDatabaseNames(
	t *testing.T,
	expected []string,
) {
	t.Helper()

	actual := testAccReadMySQLDatabaseNames(t)
	if !slices.Equal(actual, expected) {
		t.Fatalf(
			"cPanel MySQL database inventory changed: got %v, expected %v",
			actual,
			expected,
		)
	}
}
