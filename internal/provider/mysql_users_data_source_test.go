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

func TestMySQLUsersDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewMySQLUsersDataSource().Schema(
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
	users, ok := response.Schema.Attributes["users"].(datasourceschema.ListAttribute)
	if !ok || !users.Computed || users.Optional || users.Required {
		t.Fatal("users must be a computed string list")
	}
	if users.ElementType != types.StringType {
		t.Fatalf(
			"users element type = %T, want string",
			users.ElementType,
		)
	}
}

func TestAccMySQLUsersDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	userName := testAccMySQLName("idsu")
	baseline := testAccReadMySQLUserNames(t)
	t.Cleanup(func() {
		testAccRequireMySQLUserNames(t, baseline)
	})
	expected := append(slices.Clone(baseline), userName)
	slices.Sort(expected)

	const dataSourceName = "data.cpanel_mysql_users.all"
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"users.#",
			strconv.Itoa(len(expected)),
		),
	}
	for index, user := range expected {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				dataSourceName,
				fmt.Sprintf("users.%d", index),
				user,
			),
		)
	}

	config := testAccMySQLUsersDataSourceConfig(userName)
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireMySQLUserNames(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMySQLUsersDestroyed(userName),
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

func testAccMySQLUsersDataSourceConfig(userName string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "inventory_fixture" {
  name             = %q
  password         = "X8!mysqlInventoryUser-2026"
  password_version = 1
  delete_on_destroy = true
}

data "cpanel_mysql_users" "all" {
  depends_on = [cpanel_mysql_user.inventory_fixture]
}
`, userName)
}

func testAccReadMySQLUserNames(t *testing.T) []string {
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
	users, err := mysql.NewClient(client).ListUserNames(ctx)
	if err != nil {
		t.Fatalf("read cPanel MySQL user inventory: %v", err)
	}

	return users
}

func testAccRequireMySQLUserNames(
	t *testing.T,
	expected []string,
) {
	t.Helper()

	actual := testAccReadMySQLUserNames(t)
	if !slices.Equal(actual, expected) {
		t.Fatalf(
			"cPanel MySQL user inventory changed: got %v, expected %v",
			actual,
			expected,
		)
	}
}
