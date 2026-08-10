package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"testing"
	"time"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

const testAccBoxTrapperSettingsPassword = "V8!boxTrapperFixture-2026"

func TestBoxTrapperSettingsDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkdatasource.SchemaResponse{}
	NewBoxTrapperSettingsDataSource().Schema(
		t.Context(),
		frameworkdatasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	account, ok := response.Schema.Attributes["account"].(datasourceschema.StringAttribute)
	if !ok || !account.Required {
		t.Fatal("account must be a required string")
	}
	for _, name := range []string{
		"enabled",
		"enable_auto_whitelist",
		"from_addresses",
		"from_name",
		"queue_days",
		"spam_score",
		"whitelist_by_association",
	} {
		if !response.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestAccBoxTrapperSettingsResource(t *testing.T) {
	const resourceName = "cpanel_boxtrapper_settings.test"
	const fromName = "Terraform Acceptance Sender"

	address := testAccEmailAddress(t, "boxtrapper")
	var original *cpanelboxtrapper.Settings
	testAccRegisterBoxTrapperSettingsCleanup(t, address, &original)

	first := cpanelboxtrapper.Definition{
		Enabled:                true,
		EnableAutoWhitelist:    false,
		FromAddresses:          address,
		QueueDays:              9,
		SpamScore:              3.7,
		WhitelistByAssociation: false,
	}
	second := cpanelboxtrapper.Definition{
		Enabled:                false,
		EnableAutoWhitelist:    true,
		FromAddresses:          address,
		QueueDays:              12,
		SpamScore:              -1.5,
		WhitelistByAssociation: true,
	}

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckEmailAccountsDestroyed(address),
			testAccCheckBoxTrapperSettingsMissing(address),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccBoxTrapperSettingsFixtureConfig(address),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailAccountExists(address, 10),
					testAccCaptureBoxTrapperSettings(address, &original),
					testAccCheckBoxTrapperInitialSettings(
						address,
						&original,
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				Config: testAccBoxTrapperSettingsResourceConfig(
					address,
					first,
				),
				ExpectError: regexp.MustCompile(
					"(?s)cannot be changed while.*from_name as null",
				),
			},
			{
				Config: testAccBoxTrapperSettingsFixtureConfig(address),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperSettingsPointer(
						address,
						&original,
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				Config: testAccBoxTrapperStatusOnlyConfig(address, true),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperStatusOnlyState(
						resourceName,
						address,
						&original,
						true,
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				Config: testAccBoxTrapperSettingsFixtureConfig(address),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperSettingsPointer(
						address,
						&original,
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				PreConfig: func() {
					testAccPrepareBoxTrapperConfigurationFixture(
						t,
						address,
						fromName,
						&original,
					)
				},
				Config: testAccBoxTrapperSettingsResourceConfig(
					address,
					first,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperResourceState(
						resourceName,
						address,
						first,
					),
					testAccCheckBoxTrapperRestoreState(
						resourceName,
						&original,
					),
					testAccCheckNullableFromName(
						resourceName,
						testAccStringPointer(fromName),
					),
					testAccCheckBoxTrapperSettings(address, first),
					testAccCheckBoxTrapperDataSource(
						address,
						first,
						testAccStringPointer(fromName),
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				Config: testAccBoxTrapperSettingsResourceConfig(
					address,
					second,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperResourceState(
						resourceName,
						address,
						second,
					),
					testAccCheckBoxTrapperRestoreState(
						resourceName,
						&original,
					),
					testAccCheckNullableFromName(
						resourceName,
						testAccStringPointer(fromName),
					),
					testAccCheckBoxTrapperSettings(address, second),
					testAccCheckBoxTrapperDataSource(
						address,
						second,
						testAccStringPointer(fromName),
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				PreConfig: func() {
					testAccSetBoxTrapperSettings(t, address, first)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccBoxTrapperSettingsResourceConfig(
					address,
					second,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperSettings(address, second),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				Config: testAccBoxTrapperSettingsFixtureConfig(address),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperSettingsPointer(
						address,
						&original,
					),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
		},
	})
}

func TestAccBoxTrapperSettingsResourceImport(t *testing.T) {
	const resourceName = "cpanel_boxtrapper_settings.test"
	const fromName = "Terraform Import Sender"

	address := testAccEmailAddress(t, "boxtrapperimport")
	var original *cpanelboxtrapper.Settings
	testAccRegisterBoxTrapperSettingsCleanup(t, address, &original)

	target := cpanelboxtrapper.Definition{
		Enabled:                true,
		EnableAutoWhitelist:    false,
		FromAddresses:          address,
		QueueDays:              8,
		SpamScore:              2.5,
		WhitelistByAssociation: false,
	}

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckEmailAccountsDestroyed(address),
			testAccCheckBoxTrapperSettingsMissing(address),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccBoxTrapperSettingsFixtureConfig(address),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCaptureBoxTrapperSettings(address, &original),
					testAccCheckBoxTrapperQueueEmpty(address),
				),
			},
			{
				PreConfig: func() {
					testAccPrepareBoxTrapperConfigurationFixture(
						t,
						address,
						fromName,
						&original,
					)
				},
				Config: testAccBoxTrapperSettingsResourceConfig(
					address,
					target,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckBoxTrapperSettings(address, target),
					testAccCheckNullableFromName(
						resourceName,
						testAccStringPointer(fromName),
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        address,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "account",
				ImportStateVerifyIgnore: []string{
					"restore_enabled",
					"restore_enable_auto_whitelist",
					"restore_from_addresses",
					"restore_queue_days",
					"restore_spam_score",
					"restore_whitelist_by_association",
				},
			},
		},
	})
}

func testAccBoxTrapperSettingsFixtureConfig(address string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "boxtrapper" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 10
  delete_on_destroy = true
}
`, address, testAccBoxTrapperSettingsPassword)
}

func testAccBoxTrapperSettingsResourceConfig(
	address string,
	definition cpanelboxtrapper.Definition,
) string {
	return testAccBoxTrapperSettingsFixtureConfig(address) + fmt.Sprintf(`
resource "cpanel_boxtrapper_settings" "test" {
  account                  = cpanel_email_account.boxtrapper.email
  enabled                  = %t
  enable_auto_whitelist    = %t
  from_addresses           = %q
  queue_days               = %d
  spam_score               = %.1f
  whitelist_by_association = %t
}

data "cpanel_boxtrapper_settings" "test" {
  account = cpanel_boxtrapper_settings.test.account
}
`,
		definition.Enabled,
		definition.EnableAutoWhitelist,
		definition.FromAddresses,
		definition.QueueDays,
		definition.SpamScore,
		definition.WhitelistByAssociation,
	)
}

func testAccBoxTrapperStatusOnlyConfig(
	address string,
	enabled bool,
) string {
	return testAccBoxTrapperSettingsFixtureConfig(address) + fmt.Sprintf(`
resource "cpanel_boxtrapper_settings" "test" {
  account = cpanel_email_account.boxtrapper.email
  enabled = %t
}

data "cpanel_boxtrapper_settings" "test" {
  account = cpanel_boxtrapper_settings.test.account
}
`, enabled)
}

func testAccCaptureBoxTrapperSettings(
	address string,
	target **cpanelboxtrapper.Settings,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		settings, err := testAccGetBoxTrapperSettings(address)
		if err != nil {
			return err
		}
		if settings == nil {
			return fmt.Errorf(
				"BoxTrapper settings for account %q were not found",
				address,
			)
		}
		captured := *settings
		*target = &captured

		return nil
	}
}

func testAccCheckBoxTrapperResourceState(
	resourceName string,
	address string,
	definition cpanelboxtrapper.Definition,
) testresource.TestCheckFunc {
	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			address,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"enabled",
			strconv.FormatBool(definition.Enabled),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"enable_auto_whitelist",
			strconv.FormatBool(definition.EnableAutoWhitelist),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"from_addresses",
			definition.FromAddresses,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"queue_days",
			strconv.FormatInt(definition.QueueDays, 10),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"spam_score",
			strconv.FormatFloat(definition.SpamScore, 'g', -1, 64),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"whitelist_by_association",
			strconv.FormatBool(definition.WhitelistByAssociation),
		),
	)
}

func testAccCheckBoxTrapperRestoreState(
	resourceName string,
	expected **cpanelboxtrapper.Settings,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		if *expected == nil {
			return fmt.Errorf("original BoxTrapper settings were not captured")
		}
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s was not found", resourceName)
		}
		definition := (*expected).Definition()
		expectedAttributes := map[string]string{
			"restore_enabled": strconv.FormatBool(
				definition.Enabled,
			),
			"restore_enable_auto_whitelist": strconv.FormatBool(
				definition.EnableAutoWhitelist,
			),
			"restore_from_addresses": definition.FromAddresses,
			"restore_queue_days": strconv.FormatInt(
				definition.QueueDays,
				10,
			),
			"restore_spam_score": strconv.FormatFloat(
				definition.SpamScore,
				'g',
				-1,
				64,
			),
			"restore_whitelist_by_association": strconv.FormatBool(
				definition.WhitelistByAssociation,
			),
		}
		for name, expectedValue := range expectedAttributes {
			if actual := resourceState.Primary.Attributes[name]; actual != expectedValue {
				return fmt.Errorf(
					"%s.%s = %q, want %q",
					resourceName,
					name,
					actual,
					expectedValue,
				)
			}
		}

		return nil
	}
}

func testAccCheckBoxTrapperDataSource(
	address string,
	definition cpanelboxtrapper.Definition,
	fromName *string,
) testresource.TestCheckFunc {
	const dataSourceName = "data.cpanel_boxtrapper_settings.test"

	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"account",
			address,
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"enabled",
			strconv.FormatBool(definition.Enabled),
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"enable_auto_whitelist",
			strconv.FormatBool(definition.EnableAutoWhitelist),
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"from_addresses",
			definition.FromAddresses,
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"queue_days",
			strconv.FormatInt(definition.QueueDays, 10),
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"spam_score",
			strconv.FormatFloat(definition.SpamScore, 'g', -1, 64),
		),
		testresource.TestCheckResourceAttr(
			dataSourceName,
			"whitelist_by_association",
			strconv.FormatBool(definition.WhitelistByAssociation),
		),
		testAccCheckNullableFromName(
			dataSourceName,
			fromName,
		),
	)
}

func testAccCheckBoxTrapperInitialSettings(
	address string,
	expected **cpanelboxtrapper.Settings,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		if *expected == nil {
			return fmt.Errorf("initial BoxTrapper settings were not captured")
		}
		if (*expected).Enabled {
			return fmt.Errorf(
				"new BoxTrapper account %q is unexpectedly enabled",
				address,
			)
		}
		if (*expected).FromName != nil {
			return fmt.Errorf(
				"new BoxTrapper account %q from_name = %q, want null",
				address,
				*(*expected).FromName,
			)
		}

		return testAccCheckBoxTrapperSettingsPointer(
			address,
			expected,
		)(state)
	}
}

func testAccCheckBoxTrapperStatusOnlyState(
	resourceName string,
	address string,
	original **cpanelboxtrapper.Settings,
	enabled bool,
) testresource.TestCheckFunc {
	return func(state *terraform.State) error {
		if *original == nil {
			return fmt.Errorf("original BoxTrapper settings were not captured")
		}
		target := (*original).Definition()
		target.Enabled = enabled

		return testresource.ComposeAggregateTestCheckFunc(
			testAccCheckBoxTrapperResourceState(
				resourceName,
				address,
				target,
			),
			testAccCheckBoxTrapperRestoreState(resourceName, original),
			testAccCheckNullableFromName(
				resourceName,
				nil,
			),
			testAccCheckBoxTrapperSettings(address, target),
			testAccCheckBoxTrapperDataSource(address, target, nil),
		)(state)
	}
}

func testAccCheckNullableFromName(
	resourceName string,
	expected *string,
) testresource.TestCheckFunc {
	const attributeName = "from_name"

	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s was not found", resourceName)
		}
		actual, exists := resourceState.Primary.Attributes[attributeName]
		if expected == nil {
			if exists {
				return fmt.Errorf(
					"%s.%s = %q, want null",
					resourceName,
					attributeName,
					actual,
				)
			}

			return nil
		}
		if !exists || actual != *expected {
			return fmt.Errorf(
				"%s.%s = %q, want %q",
				resourceName,
				attributeName,
				actual,
				*expected,
			)
		}

		return nil
	}
}

func testAccCheckBoxTrapperSettings(
	address string,
	expected cpanelboxtrapper.Definition,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		settings, err := testAccGetBoxTrapperSettings(address)
		if err != nil {
			return err
		}
		if settings == nil {
			return fmt.Errorf(
				"BoxTrapper settings for account %q were not found",
				address,
			)
		}
		if !cpanelboxtrapper.SettingsMatchDefinition(*settings, expected) {
			return fmt.Errorf(
				"BoxTrapper settings for account %q = %#v, want %#v",
				address,
				settings.Definition(),
				expected,
			)
		}

		return nil
	}
}

func testAccCheckBoxTrapperSettingsPointer(
	address string,
	expected **cpanelboxtrapper.Settings,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		if *expected == nil {
			return fmt.Errorf("original BoxTrapper settings were not captured")
		}
		actual, err := testAccGetBoxTrapperSettings(address)
		if err != nil {
			return err
		}
		if actual == nil {
			return fmt.Errorf(
				"BoxTrapper settings for account %q were not found",
				address,
			)
		}
		if !cpanelboxtrapper.SettingsMatchDefinition(
			*actual,
			(*expected).Definition(),
		) || !testAccNullableStringsEqual(
			actual.FromName,
			(*expected).FromName,
		) {
			return fmt.Errorf(
				"BoxTrapper settings for account %q = %#v, want %#v",
				address,
				actual,
				*expected,
			)
		}

		return nil
	}
}

func testAccCheckBoxTrapperSettingsMissing(
	address string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		settings, err := testAccGetBoxTrapperSettings(address)
		if err != nil {
			return err
		}
		if settings != nil {
			return fmt.Errorf(
				"BoxTrapper settings for account %q still exist",
				address,
			)
		}

		return nil
	}
}

func testAccCheckBoxTrapperQueueEmpty(
	address string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		count, err := testAccBoxTrapperQueueCount(address)
		if err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf(
				"BoxTrapper queue for account %q contains %d message(s)",
				address,
				count,
			)
		}

		return nil
	}
}

func testAccGetBoxTrapperSettings(
	address string,
) (*cpanelboxtrapper.Settings, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return cpanelboxtrapper.NewClient(client).Get(ctx, address)
}

func testAccSetBoxTrapperSettings(
	t *testing.T,
	address string,
	definition cpanelboxtrapper.Definition,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	boxTrapperClient := cpanelboxtrapper.NewClient(client)
	current, err := boxTrapperClient.Get(ctx, address)
	if err != nil {
		t.Fatalf("read BoxTrapper settings for %q: %v", address, err)
	}
	if current == nil {
		t.Fatalf("BoxTrapper settings for %q were not found", address)
	}
	configurationTarget := definition
	configurationTarget.Enabled = current.Enabled
	if !cpanelboxtrapper.SettingsMatchDefinition(
		*current,
		configurationTarget,
	) {
		updated, _, err := boxTrapperClient.SaveConfiguration(
			ctx,
			address,
			definition,
			current.FromName,
		)
		if err != nil {
			t.Fatalf(
				"save BoxTrapper configuration for %q: %v",
				address,
				err,
			)
		}
		current = updated
	}
	if current.Enabled != definition.Enabled {
		if _, _, err := boxTrapperClient.SetStatus(
			ctx,
			address,
			definition.Enabled,
		); err != nil {
			t.Fatalf("set BoxTrapper status for %q: %v", address, err)
		}
	}
}

func testAccPrepareBoxTrapperConfigurationFixture(
	t *testing.T,
	address string,
	fromName string,
	target **cpanelboxtrapper.Settings,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	boxTrapperClient := cpanelboxtrapper.NewClient(client)
	current, err := boxTrapperClient.Get(ctx, address)
	if err != nil {
		t.Fatalf("read BoxTrapper settings for %q: %v", address, err)
	}
	if current == nil {
		t.Fatalf("BoxTrapper settings for %q were not found", address)
	}
	if current.FromName != nil && *current.FromName != fromName {
		t.Fatalf(
			"refuse to replace unexpected BoxTrapper from_name %q for %q",
			*current.FromName,
			address,
		)
	}

	if current.FromName == nil {
		response := struct{}{}
		if err := client.ExecuteUAPIOperation(
			ctx,
			http.MethodPost,
			cpanelapi.ModuleBoxTrapper,
			"save_configuration",
			map[string]string{
				"email": address,
				"enable_auto_whitelist": testAccBoxTrapperBooleanParameter(
					current.EnableAutoWhitelist,
				),
				"from_addresses": current.FromAddresses,
				"from_name":      fromName,
				"queue_days": strconv.FormatInt(
					current.QueueDays,
					10,
				),
				"spam_score": strconv.FormatFloat(
					current.SpamScore,
					'g',
					-1,
					64,
				),
				"whitelist_by_association": testAccBoxTrapperBooleanParameter(
					current.WhitelistByAssociation,
				),
			},
			&response,
		); err != nil {
			t.Fatalf(
				"set BoxTrapper fixture from_name for %q: %v",
				address,
				err,
			)
		}
	}

	updated, err := boxTrapperClient.Get(ctx, address)
	if err != nil {
		t.Fatalf(
			"verify BoxTrapper fixture from_name for %q: %v",
			address,
			err,
		)
	}
	if updated == nil ||
		updated.FromName == nil ||
		*updated.FromName != fromName ||
		!cpanelboxtrapper.SettingsMatchDefinition(
			*updated,
			current.Definition(),
		) {
		t.Fatalf(
			"BoxTrapper fixture settings for %q = %#v, want definition %#v and from_name %q",
			address,
			updated,
			current.Definition(),
			fromName,
		)
	}
	captured := *updated
	*target = &captured
}

func testAccNullableStringsEqual(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return *left == *right
}

func testAccStringPointer(value string) *string {
	return &value
}

func testAccBoxTrapperBooleanParameter(value bool) string {
	if value {
		return "1"
	}

	return "0"
}

type testAccBoxTrapperQueueResponse struct {
	Data []json.RawMessage `json:"data"`
}

func testAccBoxTrapperQueueCount(address string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return 0, err
	}
	response := testAccBoxTrapperQueueResponse{}
	if err := client.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanelapi.ModuleBoxTrapper,
		"list_queued_messages",
		map[string]string{"email": address},
		&response,
	); err != nil {
		return 0, err
	}

	return len(response.Data), nil
}

func testAccRegisterBoxTrapperSettingsCleanup(
	t *testing.T,
	address string,
	original **cpanelboxtrapper.Settings,
) {
	t.Helper()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			t.Errorf(
				"create cPanel client during BoxTrapper cleanup: %v",
				err,
			)
			return
		}
		boxTrapperClient := cpanelboxtrapper.NewClient(client)
		current, err := boxTrapperClient.Get(ctx, address)
		if err != nil {
			t.Errorf(
				"read BoxTrapper settings for %q during cleanup: %v",
				address,
				err,
			)
			return
		}
		if current == nil {
			return
		}

		if *original != nil {
			target := (*original).Definition()
			configurationTarget := target
			configurationTarget.Enabled = current.Enabled
			if !cpanelboxtrapper.SettingsMatchDefinition(
				*current,
				configurationTarget,
			) {
				updated, _, err := boxTrapperClient.SaveConfiguration(
					ctx,
					address,
					target,
					current.FromName,
				)
				if err != nil {
					t.Errorf(
						"restore BoxTrapper configuration for %q: %v",
						address,
						err,
					)
					return
				}
				current = updated
			}
			if current.Enabled != target.Enabled {
				if _, _, err := boxTrapperClient.SetStatus(
					ctx,
					address,
					target.Enabled,
				); err != nil {
					t.Errorf(
						"restore BoxTrapper status for %q: %v",
						address,
						err,
					)
					return
				}
			}
		} else if current.Enabled {
			if _, _, err := boxTrapperClient.SetStatus(
				ctx,
				address,
				false,
			); err != nil {
				t.Errorf(
					"disable BoxTrapper for %q during cleanup: %v",
					address,
					err,
				)
				return
			}
		}

		queueCount, err := testAccBoxTrapperQueueCount(address)
		if err != nil {
			t.Errorf(
				"read BoxTrapper queue for %q during cleanup: %v",
				address,
				err,
			)
			return
		}
		if queueCount != 0 {
			t.Errorf(
				"refusing to delete BoxTrapper test account %q with %d queued message(s)",
				address,
				queueCount,
			)
			return
		}

		user, domain, err := splitEmailAccountAddress(address)
		if err != nil {
			t.Errorf("split email account address %q: %v", address, err)
			return
		}
		queueCount, err = testAccBoxTrapperQueueCount(address)
		if err != nil {
			t.Errorf(
				"recheck BoxTrapper queue for %q before deletion: %v",
				address,
				err,
			)
			return
		}
		if queueCount != 0 {
			t.Errorf(
				"refusing to delete BoxTrapper test account %q after final queue recheck found %d message(s)",
				address,
				queueCount,
			)
			return
		}
		if err := cpanelmail.NewClient(client).DeleteAccount(
			ctx,
			user,
			domain,
		); err != nil {
			t.Errorf(
				"delete BoxTrapper test account %q: %v",
				address,
				err,
			)
		}
	})
}
