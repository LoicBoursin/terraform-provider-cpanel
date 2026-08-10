package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestPostgreSQLDatabaseDeletePreservesRemoteDatabaseByDefault(
	t *testing.T,
) {
	t.Parallel()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewPostgreSQLDatabaseResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &PostgreSQLDatabaseModel{
		Name:            types.StringValue("account_database"),
		Users:           types.SetValueMust(types.StringType, nil),
		DeleteOnDestroy: types.BoolValue(false),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.DeleteResponse{}
	(&postgreSQLDatabaseResource{}).Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if response.Diagnostics.WarningsCount() != 1 {
		t.Fatalf(
			"Delete() warning count = %d, want 1",
			response.Diagnostics.WarningsCount(),
		)
	}
}

func TestAccPostgreSQLDatabaseResource(t *testing.T) {
	const resourceName = "cpanel_postgresql_database.test"

	databaseName := testAccPostgreSQLName("d")
	renamedDatabaseName := testAccRegisterArtifact(databaseName + "r")
	firstUserName := testAccPostgreSQLName("du")
	secondUserName := testAccPostgreSQLName("du")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckPostgreSQLDatabasesDestroyed(
				databaseName,
				renamedDatabaseName,
			),
			testAccCheckPostgreSQLUsersDestroyed(
				firstUserName,
				secondUserName,
			),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"first"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", databaseName),
					resource.TestCheckResourceAttr(resourceName, "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", firstUserName),
					testAccCheckPostgreSQLDatabaseExists(databaseName, firstUserName),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        databaseName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"delete_on_destroy"},
			},
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"first", "second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "2"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", firstUserName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckPostgreSQLDatabaseExists(
						databaseName,
						firstUserName,
						secondUserName,
					),
				),
			},
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckPostgreSQLDatabaseExists(databaseName, secondUserName),
				),
			},
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "0"),
					testAccCheckPostgreSQLDatabaseExists(databaseName),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        databaseName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"delete_on_destroy"},
			},
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					renamedDatabaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", renamedDatabaseName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckPostgreSQLDatabaseExists(renamedDatabaseName, secondUserName),
				),
			},
			{
				PreConfig: func() {
					testAccDeletePostgreSQLDatabase(t, renamedDatabaseName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccPostgreSQLDatabaseResourceConfig(
					renamedDatabaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: testAccCheckPostgreSQLDatabaseExists(
					renamedDatabaseName,
					secondUserName,
				),
			},
		},
	})
}

func testAccPostgreSQLDatabaseResourceConfig(
	databaseName string,
	firstUserName string,
	secondUserName string,
	databaseUsers []string,
) string {
	userReferences := make([]string, 0, len(databaseUsers))
	for _, databaseUser := range databaseUsers {
		userReferences = append(
			userReferences,
			fmt.Sprintf("cpanel_postgresql_user.%s.name", databaseUser),
		)
	}

	return providerConfig + fmt.Sprintf(`
resource "cpanel_postgresql_user" "first" {
  name             = %q
  password         = "P8!firstDatabaseUser-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_postgresql_user" "second" {
  name             = %q
  password         = "R9!secondDatabaseUser-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_postgresql_database" "test" {
  name              = %q
  users             = [%s]
  delete_on_destroy = true
}
`, firstUserName, secondUserName, databaseName, joinTestAccReferences(userReferences))
}

func joinTestAccReferences(references []string) string {
	result := ""
	for index, reference := range references {
		if index > 0 {
			result += ", "
		}
		result += reference
	}

	return result
}
