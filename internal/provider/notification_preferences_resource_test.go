package provider

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelcontact "terraform-provider-cpanel/internal/cpanel/contactinformation"
)

func TestAccNotificationPreferencesResource(t *testing.T) {
	original := testAccNotificationPreferencesOriginal(t)
	first, second := testAccNotificationPreferenceTargets(original)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckNotificationPreferencesRemote(
			original.Definition(),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccNotificationPreferencesResourceConfig(first),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckNotificationPreferencesResource(
						first,
						original,
					),
					testAccCheckNotificationPreferencesDataSource(first),
					testAccCheckNotificationPreferences(first),
				),
			},
			{
				Config: testAccNotificationPreferencesResourceConfig(second),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckNotificationPreferencesResource(
						second,
						original,
					),
					testAccCheckNotificationPreferencesDataSource(second),
					testAccCheckNotificationPreferences(second),
				),
			},
			{
				PreConfig: func() {
					testAccSetNotificationPreferences(t, first)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccNotificationPreferencesResourceConfig(second),
				Check:  testAccCheckNotificationPreferences(second),
			},
		},
	})
}

func TestAccNotificationPreferencesResourceImport(t *testing.T) {
	original := testAccNotificationPreferencesOriginal(t)
	target, _ := testAccNotificationPreferenceTargets(original)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckNotificationPreferencesRemote(
			original.Definition(),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccNotificationPreferencesResourceConfig(
					original.Definition(),
				),
				Check: testAccCheckNotificationPreferencesResource(
					original.Definition(),
					original,
				),
			},
			{
				ResourceName:                         "cpanel_notification_preferences.test",
				ImportStateId:                        cpanelcontact.AccountIdentity,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "account",
			},
			{
				Config: testAccNotificationPreferencesResourceConfig(target),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckNotificationPreferencesResource(
						target,
						original,
					),
					testAccCheckNotificationPreferences(target),
				),
			},
		},
	})
}

func testAccNotificationPreferencesResourceConfig(
	preferences map[string]bool,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_notification_preferences" "test" {
  preferences = %s
}

data "cpanel_notification_preferences" "test" {
  depends_on = [cpanel_notification_preferences.test]
}
`, terraformBoolMap(preferences))
}

func terraformBoolMap(values map[string]bool) string {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)

	var builder strings.Builder
	builder.WriteString("{\n")
	for _, name := range names {
		fmt.Fprintf(
			&builder,
			"    %s = %t\n",
			name,
			values[name],
		)
	}
	builder.WriteString("  }")

	return builder.String()
}

func testAccNotificationPreferencesOriginal(
	t *testing.T,
) cpanelcontact.NotificationPreferences {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	client := testAccNotificationPreferencesClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	original, err := client.GetNotificationPreferences(ctx)
	if err != nil {
		t.Fatalf("read cPanel notification preferences: %v", err)
	}
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cleanupCancel()

		current, cleanupErr := client.GetNotificationPreferences(cleanupContext)
		if cleanupErr != nil {
			t.Errorf(
				"read cPanel notification preferences during cleanup: %v",
				cleanupErr,
			)
			return
		}
		if cpanelcontact.PreferencesMatchDefinition(
			*current,
			original.Definition(),
		) {
			return
		}
		if _, cleanupErr = client.SetNotificationPreferences(
			cleanupContext,
			original.Definition(),
		); cleanupErr != nil {
			t.Errorf(
				"restore cPanel notification preferences during cleanup: %v",
				cleanupErr,
			)
		}
	})

	return *original
}

func testAccNotificationPreferenceTargets(
	original cpanelcontact.NotificationPreferences,
) (map[string]bool, map[string]bool) {
	names := make([]string, 0, len(original.Preferences))
	for name := range original.Preferences {
		names = append(names, name)
	}
	slices.Sort(names)
	if len(names) < 2 {
		panic("cPanel account exposes fewer than two notification preferences")
	}

	first := original.Definition()
	first[names[0]] = !first[names[0]]
	second := original.Definition()
	second[names[1]] = !second[names[1]]

	return first, second
}

func testAccCheckNotificationPreferencesResource(
	expected map[string]bool,
	restore cpanelcontact.NotificationPreferences,
) testresource.TestCheckFunc {
	const resourceName = "cpanel_notification_preferences.test"

	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			cpanelcontact.AccountIdentity,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"preferences.%",
			strconv.Itoa(len(expected)),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_preferences.%",
			strconv.Itoa(len(restore.Preferences)),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"descriptions.%",
			strconv.Itoa(len(expected)),
		),
	}
	for name, enabled := range expected {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				"preferences."+name,
				strconv.FormatBool(enabled),
			),
			testresource.TestCheckResourceAttrSet(
				resourceName,
				"descriptions."+name,
			),
		)
	}
	for name, enabled := range restore.Preferences {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				"restore_preferences."+name,
				strconv.FormatBool(enabled),
			),
		)
	}

	return testresource.ComposeAggregateTestCheckFunc(checks...)
}

func testAccCheckNotificationPreferencesDataSource(
	expected map[string]bool,
) testresource.TestCheckFunc {
	const resourceName = "data.cpanel_notification_preferences.test"

	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			cpanelcontact.AccountIdentity,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"preferences.%",
			strconv.Itoa(len(expected)),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"descriptions.%",
			strconv.Itoa(len(expected)),
		),
	}
	for name, enabled := range expected {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				"preferences."+name,
				strconv.FormatBool(enabled),
			),
			testresource.TestCheckResourceAttrSet(
				resourceName,
				"descriptions."+name,
			),
		)
	}

	return testresource.ComposeAggregateTestCheckFunc(checks...)
}

func testAccCheckNotificationPreferences(
	expected map[string]bool,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		actual, err := testAccReadNotificationPreferences()
		if err != nil {
			return err
		}
		if !cpanelcontact.PreferencesMatchDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel notification preferences = %#v, want %#v",
				actual.Preferences,
				expected,
			)
		}

		return nil
	}
}

func testAccCheckNotificationPreferencesRemote(
	expected map[string]bool,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		actual, err := testAccReadNotificationPreferences()
		if err != nil {
			return err
		}
		if !cpanelcontact.PreferencesMatchDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel notification preferences = %#v, want %#v",
				actual.Preferences,
				expected,
			)
		}

		return nil
	}
}

func testAccReadNotificationPreferences() (*cpanelcontact.NotificationPreferences, error) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return cpanelcontact.NewClient(client).
		GetNotificationPreferences(ctx)
}

func testAccSetNotificationPreferences(
	t *testing.T,
	definition map[string]bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if _, err := testAccNotificationPreferencesClient(t).
		SetNotificationPreferences(ctx, definition); err != nil {
		t.Fatalf("set cPanel notification preferences: %v", err)
	}
}

func testAccNotificationPreferencesClient(
	t *testing.T,
) *cpanelcontact.Client {
	t.Helper()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	return cpanelcontact.NewClient(client)
}
