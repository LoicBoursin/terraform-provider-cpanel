package provider

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testingresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

type fakePostgreSQLUserClient struct {
	existsByName map[string]bool
	existsErr    error
	passwordErr  error
}

func (c *fakePostgreSQLUserClient) CreateUser(
	context.Context,
	postgresql.UserCreateModel,
) (*postgresql.UserDataSourceModel, error) {
	return &postgresql.UserDataSourceModel{}, nil
}

func (c *fakePostgreSQLUserClient) DeleteUser(
	_ context.Context,
	input postgresql.UserDeleteModel,
) (*postgresql.UserDataSourceModel, error) {
	delete(c.existsByName, input.Name)

	return &postgresql.UserDataSourceModel{}, nil
}

func (c *fakePostgreSQLUserClient) SetPassword(
	context.Context,
	postgresql.UserSetPasswordModel,
) (*postgresql.UserDataSourceModel, error) {
	if c.passwordErr != nil {
		return nil, c.passwordErr
	}

	return &postgresql.UserDataSourceModel{}, nil
}

func (c *fakePostgreSQLUserClient) UserExists(
	_ context.Context,
	name string,
) (bool, error) {
	if c.existsErr != nil {
		return false, c.existsErr
	}

	return c.existsByName[name], nil
}

func TestPostgreSQLUserVerification(t *testing.T) {
	t.Parallel()

	client := &fakePostgreSQLUserClient{
		existsByName: map[string]bool{
			"account_new": true,
		},
	}
	resource := &postgreSQLUserResource{client: client}

	if err := resource.verifyUser(
		context.Background(),
		"account_new",
	); err != nil {
		t.Fatalf("verifyUser() error = %v, want nil", err)
	}
	if err := resource.verifyUser(
		context.Background(),
		"account_old",
	); err == nil {
		t.Fatal("verifyUser() error = nil, want missing-user error")
	}
}

func TestPostgreSQLUserNameRequiresReplacement(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewPostgreSQLUserResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	name, ok := response.Schema.Attributes["name"].(resourceschema.StringAttribute)
	if !ok || len(name.PlanModifiers) == 0 {
		t.Fatal("PostgreSQL user name must require replacement")
	}
}

func TestPostgreSQLUserUpgradeConfigurationChoices(t *testing.T) {
	testingresource.UnitTest(t, testingresource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testingresource.TestStep{
			{
				Config: postgreSQLUserUpgradeTestConfig(`
  password = "existing-config-password"
`),
				ExpectError: regexp.MustCompile(`password_version`),
			},
			{
				Config:             postgreSQLUserUpgradeTestConfig(""),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: postgreSQLUserUpgradeTestConfig(`
  password         = "existing-config-password"
  password_version = 1
`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func postgreSQLUserUpgradeTestConfig(passwordConfiguration string) string {
	return `
provider "cpanel" {
  host           = "https://cpanel.example.invalid:2083"
  username       = "account"
  api_token      = "unit-test-token"
  api_token_name = "unit-test-token-name"
}

resource "cpanel_postgresql_user" "test" {
  name = "account_user"
` + passwordConfiguration + `}
`
}

func TestPostgreSQLAmbiguousMutationErrorRedactsPassword(t *testing.T) {
	t.Parallel()

	const password = "do-not-leak-this-password"
	detail := postgreSQLAmbiguousMutationErrorDetail(
		errors.New("connection reset after sending "+password),
		"PostgreSQL user creation",
		errors.New("user exists"),
	)
	if strings.Contains(detail, password) {
		t.Fatalf("ambiguous mutation detail leaked %q", password)
	}
}

func TestPostgreSQLMutationErrorClassification(t *testing.T) {
	t.Parallel()

	if !postgreSQLMutationErrorIsDeterministic(
		&cpanelapi.APIError{
			API:      "UAPI",
			Module:   "Postgresql",
			Function: "create_user",
		},
	) {
		t.Fatal("API error must be deterministic")
	}
	if postgreSQLMutationErrorIsDeterministic(
		errors.New("connection reset"),
	) {
		t.Fatal("transport error must be ambiguous")
	}
}

func TestPostgreSQLUserStateUpgradeRemovesStoredPassword(t *testing.T) {
	t.Parallel()

	resourceUnderTest := &postgreSQLUserResource{}
	schemaResponse := &frameworkresource.SchemaResponse{}
	resourceUnderTest.Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	upgrader, ok := resourceUnderTest.UpgradeState(t.Context())[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("PostgreSQL user state upgrader for version 0 is missing")
	}

	priorState := tfsdk.State{Schema: *upgrader.PriorSchema}
	diagnostics := priorState.Set(t.Context(), &postgreSQLUserModelV0{
		Name:        types.StringValue("account_user"),
		Password:    types.StringValue("legacy-state-password"),
		LastUpdated: types.StringValue("2025-01-01T00:00:00Z"),
	})
	if diagnostics.HasError() {
		t.Fatalf("prior State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.UpgradeStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	upgrader.StateUpgrader(
		t.Context(),
		frameworkresource.UpgradeStateRequest{State: &priorState},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("StateUpgrader() diagnostics: %v", response.Diagnostics)
	}

	var upgraded PostgreSQLUserModel
	diagnostics = response.State.Get(t.Context(), &upgraded)
	if diagnostics.HasError() {
		t.Fatalf("upgraded State.Get() diagnostics: %v", diagnostics)
	}
	if got, want := upgraded.Name.ValueString(), "account_user"; got != want {
		t.Fatalf("upgraded name = %q; want %q", got, want)
	}
	if !upgraded.Password.IsNull() || !upgraded.PasswordVersion.IsNull() {
		t.Fatal("upgraded state retained password data")
	}
	if upgraded.DeleteOnDestroy.IsNull() ||
		upgraded.DeleteOnDestroy.IsUnknown() ||
		upgraded.DeleteOnDestroy.ValueBool() {
		t.Fatal("upgraded state must preserve the remote user by default")
	}
}

func TestPostgreSQLUserDeletePreservesRemoteUserByDefault(t *testing.T) {
	t.Parallel()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewPostgreSQLUserResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &PostgreSQLUserModel{
		Name:            types.StringValue("account_user"),
		Password:        types.StringNull(),
		PasswordVersion: types.Int64Value(1),
		DeleteOnDestroy: types.BoolValue(false),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.DeleteResponse{}
	(&postgreSQLUserResource{}).Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if response.Diagnostics.WarningsCount() != 1 {
		t.Fatalf(
			"Delete() warnings = %d; want 1",
			response.Diagnostics.WarningsCount(),
		)
	}
}
