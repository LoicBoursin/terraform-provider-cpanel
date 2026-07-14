package provider

import (
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestReconcileMySQLDatabaseRenamePresence(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		oldExists   bool
		newExists   bool
		wantApplied bool
		wantError   bool
	}{
		"new identity cannot be attributed": {
			newExists: true,
			wantError: true,
		},
		"both names exist": {
			oldExists: true,
			newExists: true,
			wantError: true,
		},
		"not applied": {
			oldExists: true,
		},
		"lost both names": {
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			applied, err :=
				reconcileMySQLDatabaseRenamePresence(
					test.oldExists,
					test.newExists,
				)
			if test.wantError && err == nil {
				t.Fatal("reconcileMySQLDatabaseRenamePresence() error = nil, want error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"reconcileMySQLDatabaseRenamePresence() error = %v",
					err,
				)
			}
			if applied != test.wantApplied {
				t.Fatalf(
					"reconcileMySQLDatabaseRenamePresence() = %t, want %t",
					applied,
					test.wantApplied,
				)
			}
		})
	}
}

func TestMySQLDatabaseDeletePreservesRemoteDatabaseByDefault(t *testing.T) {
	t.Parallel()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewMySQLDatabaseResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &MySQLDatabaseModel{
		Name:            types.StringValue("account_database"),
		Users:           types.SetValueMust(types.StringType, nil),
		DeleteOnDestroy: types.BoolValue(false),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.DeleteResponse{}
	(&mySQLDatabaseResource{}).Delete(
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

func TestAccMySQLDatabaseResource(t *testing.T) {
	const resourceName = "cpanel_mysql_database.test"

	databaseName := testAccMySQLName("d")
	renamedDatabaseName := testAccRegisterArtifact(databaseName + "r")
	firstUserName := testAccMySQLName("du")
	secondUserName := testAccMySQLName("du")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckMySQLDatabasesDestroyed(
				databaseName,
				renamedDatabaseName,
			),
			testAccCheckMySQLUsersDestroyed(
				firstUserName,
				secondUserName,
			),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccMySQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"first"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", databaseName),
					resource.TestCheckResourceAttr(resourceName, "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", firstUserName),
					testAccCheckMySQLDatabaseExists(databaseName, firstUserName),
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
				Config: testAccMySQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"first", "second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "2"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", firstUserName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckMySQLDatabaseExists(
						databaseName,
						firstUserName,
						secondUserName,
					),
				),
			},
			{
				Config: testAccMySQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "1"),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckMySQLDatabaseExists(databaseName, secondUserName),
				),
			},
			{
				Config: testAccMySQLDatabaseResourceConfig(
					databaseName,
					firstUserName,
					secondUserName,
					[]string{},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "users.#", "0"),
					testAccCheckMySQLDatabaseExists(databaseName),
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
				Config: testAccMySQLDatabaseResourceConfig(
					renamedDatabaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", renamedDatabaseName),
					resource.TestCheckTypeSetElemAttr(resourceName, "users.*", secondUserName),
					testAccCheckMySQLDatabaseExists(renamedDatabaseName, secondUserName),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteMySQLDatabase(t, renamedDatabaseName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccMySQLDatabaseResourceConfig(
					renamedDatabaseName,
					firstUserName,
					secondUserName,
					[]string{"second"},
				),
				Check: testAccCheckMySQLDatabaseExists(
					renamedDatabaseName,
					secondUserName,
				),
			},
		},
	})
}

func testAccMySQLDatabaseResourceConfig(
	databaseName string,
	firstUserName string,
	secondUserName string,
	databaseUsers []string,
) string {
	userReferences := make([]string, 0, len(databaseUsers))
	for _, databaseUser := range databaseUsers {
		userReferences = append(
			userReferences,
			fmt.Sprintf("cpanel_mysql_user.%s.name", databaseUser),
		)
	}

	return providerConfig + fmt.Sprintf(`
resource "cpanel_mysql_user" "first" {
  name             = %q
  password         = "P8!firstMySQLDatabaseUser-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_mysql_user" "second" {
  name             = %q
  password         = "R9!secondMySQLDatabaseUser-2026"
  password_version = 1
  delete_on_destroy = true
}

resource "cpanel_mysql_database" "test" {
  name              = %q
  users             = [%s]
  delete_on_destroy = true
}
`, firstUserName, secondUserName, databaseName, joinTestAccReferences(userReferences))
}
