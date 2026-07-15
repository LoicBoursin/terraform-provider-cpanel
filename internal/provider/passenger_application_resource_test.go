package provider

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelpassenger "terraform-provider-cpanel/internal/cpanel/passenger"
)

func TestPassengerApplicationResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewPassengerApplicationResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"name", "path", "domain"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required string", attributeName)
		}
	}

	baseURI, ok := response.Schema.Attributes["base_uri"].(resourceschema.StringAttribute)
	if !ok || !baseURI.Optional || !baseURI.Computed ||
		len(baseURI.PlanModifiers) == 0 {
		t.Fatal(
			"base_uri must be optional, computed, and require replacement",
		)
	}

	environment, ok := response.Schema.Attributes["environment_variables"].(resourceschema.MapAttribute)
	if !ok || !environment.Optional || !environment.Computed ||
		!environment.Sensitive || len(environment.PlanModifiers) != 1 {
		t.Fatal(
			"environment_variables must be optional, computed, sensitive, and preserve imported state when omitted",
		)
	}
}

func TestPassengerApplicationDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewPassengerApplicationDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	name, ok := response.Schema.Attributes["name"].(datasourceschema.StringAttribute)
	if !ok || !name.Required {
		t.Fatal("name must be a required string")
	}

	environment, ok := response.Schema.Attributes["environment_variables"].(datasourceschema.MapAttribute)
	if !ok || !environment.Computed || !environment.Sensitive {
		t.Fatal(
			"environment_variables must be computed and sensitive",
		)
	}
}

func TestAccPassengerApplicationResource(t *testing.T) {
	const resourceName = "cpanel_passenger_application.test"

	initialName := testAccPassengerApplicationName("initial")
	updatedName := testAccPassengerApplicationName("updated")
	initialRoot := testAccGitRepositoryRoot("passenger-initial")
	replacementRoot := testAccGitRepositoryRoot("passenger-replacement")
	initialBaseURI := testAccPassengerApplicationBaseURI("initial")
	replacementBaseURI := testAccPassengerApplicationBaseURI("replacement")
	domain := testAccMainDomain(t)
	initialEnvironment := map[string]string{
		"TFCPANEL_FIRST":  "initial",
		"TFCPANEL_SECOND": "one",
	}
	updatedEnvironment := map[string]string{
		"TFCPANEL_FIRST": "updated",
		"TFCPANEL_THIRD": "three",
	}

	initialDefinition := cpanelpassenger.Definition{
		Name:                 initialName,
		Path:                 initialRoot,
		Domain:               domain,
		BaseURI:              initialBaseURI,
		DeploymentMode:       cpanelpassenger.DeploymentModeProduction,
		Enabled:              false,
		EnvironmentVariables: initialEnvironment,
	}
	updatedDefinition := cpanelpassenger.Definition{
		Name:                 updatedName,
		Path:                 replacementRoot,
		Domain:               domain,
		BaseURI:              initialBaseURI,
		DeploymentMode:       cpanelpassenger.DeploymentModeDevelopment,
		Enabled:              true,
		EnvironmentVariables: updatedEnvironment,
	}
	emptyEnvironmentDefinition := updatedDefinition
	emptyEnvironmentDefinition.Enabled = false
	emptyEnvironmentDefinition.EnvironmentVariables = map[string]string{}
	preservedEnvironmentDefinition := updatedDefinition
	preservedEnvironmentDefinition.Enabled = false
	replacementDefinition := emptyEnvironmentDefinition
	replacementDefinition.BaseURI = replacementBaseURI

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeAggregateTestCheckFunc(
			testAccCheckPassengerApplicationsDestroyed(
				initialName,
				updatedName,
			),
			testAccCheckGitRepositoriesDestroyed(
				initialRoot,
				replacementRoot,
			),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					initialDefinition,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"path",
						initialRoot,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"deployment_mode",
						cpanelpassenger.DeploymentModeProduction,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"enabled",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"environment_variables.TFCPANEL_FIRST",
						"initial",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_path",
					),
					testAccCheckPassengerApplicationExists(
						initialDefinition,
					),
				),
			},
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					updatedDefinition,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckPassengerApplicationMissing(initialName),
					testAccCheckPassengerApplicationExists(
						updatedDefinition,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        updatedName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: testAccPassengerApplicationResourceConfigWithoutEnvironment(
					initialRoot,
					replacementRoot,
					preservedEnvironmentDefinition,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"enabled",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"environment_variables.TFCPANEL_FIRST",
						"updated",
					),
					testAccCheckPassengerApplicationExists(
						preservedEnvironmentDefinition,
					),
				),
			},
			{
				PreConfig: func() {
					testAccDriftPassengerApplication(t, updatedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					updatedDefinition,
				),
				Check: testAccCheckPassengerApplicationExists(
					updatedDefinition,
				),
			},
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					emptyEnvironmentDefinition,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"environment_variables.%",
						"0",
					),
					testAccCheckPassengerApplicationExists(
						emptyEnvironmentDefinition,
					),
				),
			},
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					replacementDefinition,
				),
				Check: testAccCheckPassengerApplicationExists(
					replacementDefinition,
				),
			},
			{
				PreConfig: func() {
					testAccDeletePassengerApplication(t, updatedName)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccPassengerApplicationResourceConfig(
					initialRoot,
					replacementRoot,
					replacementDefinition,
				),
				Check: testAccCheckPassengerApplicationExists(
					replacementDefinition,
				),
			},
		},
	})
}

func TestAccPassengerApplicationDataSource(t *testing.T) {
	const (
		resourceName   = "cpanel_passenger_application.test"
		dataSourceName = "data.cpanel_passenger_application.test"
	)

	name := testAccPassengerApplicationName("data")
	repositoryRoot := testAccGitRepositoryRoot("passenger-data")
	definition := cpanelpassenger.Definition{
		Name:           name,
		Path:           repositoryRoot,
		Domain:         testAccMainDomain(t),
		BaseURI:        testAccPassengerApplicationBaseURI("data"),
		DeploymentMode: cpanelpassenger.DeploymentModeProduction,
		Enabled:        false,
		EnvironmentVariables: map[string]string{
			"TFCPANEL_DATA": "source",
		},
	}

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeAggregateTestCheckFunc(
			testAccCheckPassengerApplicationsDestroyed(name),
			testAccCheckGitRepositoriesDestroyed(repositoryRoot),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccPassengerApplicationDataSourceConfig(
					repositoryRoot,
					definition,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"name",
						name,
					),
					testresource.TestCheckResourceAttrPair(
						dataSourceName,
						"path",
						resourceName,
						"path",
					),
					testresource.TestCheckResourceAttrPair(
						dataSourceName,
						"absolute_path",
						resourceName,
						"absolute_path",
					),
					testresource.TestCheckResourceAttr(
						dataSourceName,
						"environment_variables.TFCPANEL_DATA",
						"source",
					),
				),
			},
		},
	})
}

func testAccPassengerApplicationName(kind string) string {
	return testAccRegisterArtifact(fmt.Sprintf(
		"tfcpanelpassenger%s%s",
		strings.ToLower(kind),
		strings.ToLower(
			acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
		),
	))
}

func testAccPassengerApplicationBaseURI(kind string) string {
	return fmt.Sprintf(
		"/tfcpanelpassenger-%s-%s",
		strings.ToLower(kind),
		strings.ToLower(
			acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum),
		),
	)
}

func testAccPassengerApplicationResourceConfig(
	initialRoot string,
	replacementRoot string,
	definition cpanelpassenger.Definition,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "initial" {
  name                       = "Terraform Passenger initial fixture"
  repository_root            = %q
  delete_contents_on_destroy = true
}

resource "cpanel_git_repository" "replacement" {
  name                       = "Terraform Passenger replacement fixture"
  repository_root            = %q
  delete_contents_on_destroy = true

  depends_on = [cpanel_git_repository.initial]
}

resource "cpanel_passenger_application" "test" {
  name                  = %q
  path                  = %q
  domain                = %q
  base_uri              = %q
  deployment_mode       = %q
  enabled               = %t
  environment_variables = %s

  depends_on = [
    cpanel_git_repository.initial,
    cpanel_git_repository.replacement,
  ]
}
`,
		initialRoot,
		replacementRoot,
		definition.Name,
		definition.Path,
		definition.Domain,
		definition.BaseURI,
		definition.DeploymentMode,
		definition.Enabled,
		testAccPassengerEnvironmentVariables(definition.EnvironmentVariables),
	)
}

func testAccPassengerApplicationResourceConfigWithoutEnvironment(
	initialRoot string,
	replacementRoot string,
	definition cpanelpassenger.Definition,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "initial" {
  name                       = "Terraform Passenger initial fixture"
  repository_root            = %q
  delete_contents_on_destroy = true
}

resource "cpanel_git_repository" "replacement" {
  name                       = "Terraform Passenger replacement fixture"
  repository_root            = %q
  delete_contents_on_destroy = true

  depends_on = [cpanel_git_repository.initial]
}

resource "cpanel_passenger_application" "test" {
  name            = %q
  path            = %q
  domain          = %q
  base_uri        = %q
  deployment_mode = %q
  enabled         = %t

  depends_on = [
    cpanel_git_repository.initial,
    cpanel_git_repository.replacement,
  ]
}
`,
		initialRoot,
		replacementRoot,
		definition.Name,
		definition.Path,
		definition.Domain,
		definition.BaseURI,
		definition.DeploymentMode,
		definition.Enabled,
	)
}

func testAccPassengerApplicationDataSourceConfig(
	repositoryRoot string,
	definition cpanelpassenger.Definition,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = "Terraform Passenger data source fixture"
  repository_root            = %q
  delete_contents_on_destroy = true
}

resource "cpanel_passenger_application" "test" {
  name                  = %q
  path                  = cpanel_git_repository.test.repository_root
  domain                = %q
  base_uri              = %q
  deployment_mode       = %q
  enabled               = %t
  environment_variables = %s
}

data "cpanel_passenger_application" "test" {
  name = cpanel_passenger_application.test.name
}
`,
		repositoryRoot,
		definition.Name,
		definition.Domain,
		definition.BaseURI,
		definition.DeploymentMode,
		definition.Enabled,
		testAccPassengerEnvironmentVariables(definition.EnvironmentVariables),
	)
}

func testAccPassengerEnvironmentVariables(values map[string]string) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	var builder strings.Builder
	builder.WriteString("{\n")
	for _, name := range names {
		fmt.Fprintf(&builder, "    %s = %q\n", name, values[name])
	}
	builder.WriteString("  }")

	return builder.String()
}

func testAccCheckPassengerApplicationExists(
	expected cpanelpassenger.Definition,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		application, err := cpanelpassenger.NewClient(client).Get(
			ctx,
			expected.Name,
		)
		if err != nil {
			return err
		}
		if application == nil {
			return fmt.Errorf(
				"Passenger application %q was not found",
				expected.Name,
			)
		}
		if !application.Matches(expected) {
			return fmt.Errorf(
				"Passenger application %q is %#v; want %#v",
				expected.Name,
				application.Definition(),
				expected,
			)
		}

		return nil
	}
}

func testAccCheckPassengerApplicationMissing(
	name string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		application, err := cpanelpassenger.NewClient(client).Get(ctx, name)
		if err != nil {
			return err
		}
		if application != nil {
			return fmt.Errorf(
				"Passenger application %q still exists",
				name,
			)
		}

		return nil
	}
}

func testAccCheckPassengerApplicationsDestroyed(
	names ...string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		passengerClient := cpanelpassenger.NewClient(client)
		var validationErr error
		for _, name := range names {
			application, err := passengerClient.Get(ctx, name)
			if err != nil && validationErr == nil {
				validationErr = err
			}
			if application == nil {
				continue
			}
			if validationErr == nil {
				validationErr = fmt.Errorf(
					"Passenger application %q still exists after destroy",
					name,
				)
			}
			if err := passengerClient.Delete(
				ctx,
				name,
			); err != nil && validationErr == nil {
				validationErr = err
			}
		}

		return validationErr
	}
}

func testAccDriftPassengerApplication(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	passengerClient := cpanelpassenger.NewClient(client)
	application, err := passengerClient.Get(ctx, name)
	if err != nil {
		t.Fatalf("read Passenger application %q: %v", name, err)
	}
	if application == nil {
		t.Fatalf("Passenger application %q was not found", name)
	}
	definition := application.Definition()
	definition.DeploymentMode = cpanelpassenger.DeploymentModeProduction
	definition.Enabled = false
	definition.EnvironmentVariables = map[string]string{
		"TFCPANEL_DRIFT": "external",
	}
	if err := passengerClient.Update(
		ctx,
		name,
		definition,
	); err != nil {
		t.Fatalf("drift Passenger application %q: %v", name, err)
	}
}

func testAccDeletePassengerApplication(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	passengerClient := cpanelpassenger.NewClient(client)
	application, err := passengerClient.Get(ctx, name)
	if err != nil {
		t.Fatalf("read Passenger application %q: %v", name, err)
	}
	if application == nil {
		return
	}
	if err := passengerClient.Delete(ctx, name); err != nil {
		t.Fatalf("delete Passenger application %q: %v", name, err)
	}
}
