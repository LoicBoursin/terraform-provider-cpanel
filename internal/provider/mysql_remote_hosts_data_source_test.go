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

const testAccMySQLRemoteHostsInventoryFixture = "198.51.100.244"

func TestMySQLRemoteHostsDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewMySQLRemoteHostsDataSource().Schema(
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
	hosts, ok := response.Schema.Attributes["hosts"].(datasourceschema.ListAttribute)
	if !ok || !hosts.Computed || hosts.Optional || hosts.Required {
		t.Fatal("hosts must be a computed string list")
	}
	if hosts.ElementType != types.StringType {
		t.Fatalf(
			"hosts element type = %T, want string",
			hosts.ElementType,
		)
	}
}

func TestAccMySQLRemoteHostsDataSource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}
	testAccRegisterArtifact(testAccMySQLRemoteHostsInventoryFixture)

	baseline := testAccReadMySQLRemoteHostNames(t)
	t.Cleanup(func() {
		testAccRequireMySQLRemoteHostNames(t, baseline)
	})
	if slices.Contains(
		baseline,
		testAccMySQLRemoteHostsInventoryFixture,
	) {
		t.Fatalf(
			"reserved remote MySQL fixture %q already exists",
			testAccMySQLRemoteHostsInventoryFixture,
		)
	}
	expected := append(
		slices.Clone(baseline),
		testAccMySQLRemoteHostsInventoryFixture,
	)
	slices.Sort(expected)

	const dataSourceName = "data.cpanel_mysql_remote_hosts.all"
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"hosts.#",
			strconv.Itoa(len(expected)),
		),
	}
	for index, host := range expected {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				dataSourceName,
				fmt.Sprintf("hosts.%d", index),
				host,
			),
		)
	}

	const config = providerConfig + `
resource "cpanel_mysql_remote_host" "inventory_fixture" {
  host = "198.51.100.244"
  note = "terraform-provider-cpanel inventory fixture"
}

data "cpanel_mysql_remote_hosts" "all" {
  depends_on = [cpanel_mysql_remote_host.inventory_fixture]
}
`
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireMySQLRemoteHostNames(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckMySQLRemoteHostsDestroyed(
			testAccMySQLRemoteHostsInventoryFixture,
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

func testAccReadMySQLRemoteHostNames(t *testing.T) []string {
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
	hosts, err := mysql.NewClient(client).ListRemoteHostNames(ctx)
	if err != nil {
		t.Fatalf("read cPanel remote MySQL host inventory: %v", err)
	}

	return hosts
}

func testAccRequireMySQLRemoteHostNames(
	t *testing.T,
	expected []string,
) {
	t.Helper()

	actual := testAccReadMySQLRemoteHostNames(t)
	if !slices.Equal(actual, expected) {
		t.Fatalf(
			"cPanel remote MySQL host inventory changed: got %v, expected %v",
			actual,
			expected,
		)
	}
}
