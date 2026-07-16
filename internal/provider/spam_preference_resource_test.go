package provider

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

func TestAccSpamPreferenceResource(t *testing.T) {
	original := testAccSpamPreferenceOriginal(
		t,
		cpanelspam.PreferenceRequiredScore,
	)
	first := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"5.5"},
		Present: true,
	}
	second := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"6.25"},
		Present: true,
	}

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckSpamPreferenceRemote(
			original.Definition(),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccSpamPreferenceResourceConfig(first),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSpamPreferenceResource(first, *original),
					testAccCheckSpamPreferenceDataSource(first),
					testAccCheckSpamPreference(first),
				),
			},
			{
				Config: testAccSpamPreferenceResourceConfig(second),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSpamPreferenceResource(second, *original),
					testAccCheckSpamPreferenceDataSource(second),
					testAccCheckSpamPreference(second),
				),
			},
			{
				PreConfig: func() {
					testAccApplySpamPreference(
						t,
						cpanelspam.Definition{
							Name:    second.Name,
							Values:  []string{"7.75"},
							Present: true,
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccSpamPreferenceResourceConfig(second),
				Check:  testAccCheckSpamPreference(second),
			},
		},
	})
}

func TestAccSpamPreferenceResourceImport(t *testing.T) {
	const preference = cpanelspam.PreferenceRequiredScore
	original := testAccSpamPreferenceOriginal(t, preference)
	target := cpanelspam.Definition{
		Name:    preference,
		Values:  []string{"5.5"},
		Present: true,
	}
	updated := cpanelspam.Definition{
		Name:    preference,
		Values:  []string{"6.25"},
		Present: true,
	}

	testAccApplySpamPreference(t, target)
	t.Cleanup(func() {
		testAccApplySpamPreference(t, original.Definition())
	})

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSpamPreferenceRemote(target),
		Steps: []testresource.TestStep{
			{
				Config: testAccSpamPreferenceResourceConfig(target),
				Check:  testAccCheckSpamPreference(target),
			},
			{
				ResourceName:                         "cpanel_spam_preference.test",
				ImportStateId:                        preference,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "preference",
			},
			{
				Config: testAccSpamPreferenceResourceConfig(updated),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckSpamPreferenceResource(updated, preferenceFromDefinition(target)),
					testAccCheckSpamPreference(updated),
				),
			},
		},
	})
}

func testAccSpamPreferenceResourceConfig(
	definition cpanelspam.Definition,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_spam_preference" "test" {
  preference = %q
  values     = %s
}

data "cpanel_spam_preference" "test" {
  preference = cpanel_spam_preference.test.preference
  depends_on = [cpanel_spam_preference.test]
}
`, definition.Name, terraformStringSet(definition.Values))
}

func terraformStringSet(values []string) string {
	result := "[\n"
	for _, value := range values {
		result += fmt.Sprintf("    %q,\n", value)
	}

	return result + "  ]"
}

func testAccSpamPreferenceOriginal(
	t *testing.T,
	name string,
) *cpanelspam.Preference {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	original, err := testAccSpamPreferenceClient(t).
		GetPreference(ctx, name)
	if err != nil {
		t.Fatalf("read cPanel SpamAssassin preference: %v", err)
	}
	t.Cleanup(func() {
		testAccApplySpamPreference(t, original.Definition())
	})

	return original
}

func testAccCheckSpamPreferenceResource(
	expected cpanelspam.Definition,
	restore cpanelspam.Preference,
) testresource.TestCheckFunc {
	const resourceName = "cpanel_spam_preference.test"

	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			resourceName,
			"preference",
			expected.Name,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"configured",
			"true",
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"values.#",
			strconv.Itoa(len(expected.Values)),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_present",
			strconv.FormatBool(restore.Present),
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_values.#",
			strconv.Itoa(len(restore.Values)),
		),
	}
	for _, value := range expected.Values {
		checks = append(
			checks,
			testresource.TestCheckTypeSetElemAttr(
				resourceName,
				"values.*",
				value,
			),
		)
	}
	for _, value := range restore.Values {
		checks = append(
			checks,
			testresource.TestCheckTypeSetElemAttr(
				resourceName,
				"restore_values.*",
				value,
			),
		)
	}

	return testresource.ComposeAggregateTestCheckFunc(checks...)
}

func testAccCheckSpamPreferenceDataSource(
	expected cpanelspam.Definition,
) testresource.TestCheckFunc {
	const resourceName = "data.cpanel_spam_preference.test"

	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			resourceName,
			"preference",
			expected.Name,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"configured",
			"true",
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"values.#",
			strconv.Itoa(len(expected.Values)),
		),
	}
	for _, value := range expected.Values {
		checks = append(
			checks,
			testresource.TestCheckTypeSetElemAttr(
				resourceName,
				"values.*",
				value,
			),
		)
	}

	return testresource.ComposeAggregateTestCheckFunc(checks...)
}

func testAccCheckSpamPreference(
	expected cpanelspam.Definition,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		actual, err := testAccReadSpamPreference(expected.Name)
		if err != nil {
			return err
		}
		if !cpanelspam.PreferenceMatchesDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel SpamAssassin preference = %#v, want %#v",
				actual,
				expected,
			)
		}

		return nil
	}
}

func testAccCheckSpamPreferenceRemote(
	expected cpanelspam.Definition,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		actual, err := testAccReadSpamPreference(expected.Name)
		if err != nil {
			return err
		}
		if !cpanelspam.PreferenceMatchesDefinition(*actual, expected) {
			return fmt.Errorf(
				"cPanel SpamAssassin preference = %#v, want %#v",
				actual,
				expected,
			)
		}

		return nil
	}
}

func testAccReadSpamPreference(
	name string,
) (*cpanelspam.Preference, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}

	return cpanelspam.NewClient(client).GetPreference(ctx, name)
}

func testAccApplySpamPreference(
	t *testing.T,
	definition cpanelspam.Definition,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := testAccSpamPreferenceClient(t)
	var err error
	if definition.Present {
		_, err = client.SetPreference(
			ctx,
			definition.Name,
			definition.Values,
		)
	} else {
		_, err = client.RemovePreference(ctx, definition.Name)
	}
	if err != nil {
		t.Fatalf("apply cPanel SpamAssassin preference: %v", err)
	}
}

func testAccSpamPreferenceClient(t *testing.T) *cpanelspam.Client {
	t.Helper()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	return cpanelspam.NewClient(client)
}
