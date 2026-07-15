package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

func TestLocaleResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewLocaleResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	locale, ok := response.Schema.Attributes["locale"].(resourceschema.StringAttribute)
	if !ok || !locale.Required {
		t.Fatal("locale must be a required string")
	}
	for _, attributeName := range []string{
		"account",
		"restore_locale",
		"name",
		"local_name",
		"direction",
		"encoding",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed string", attributeName)
		}
	}
}

func TestAccLocaleResource(t *testing.T) {
	original, alternate := testAccLocalePair(t)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLocale(original.Code),
		Steps: []testresource.TestStep{
			{
				Config: testAccLocaleResourceConfig(alternate.Code),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLocaleResource(
						alternate,
						original.Code,
					),
					testAccCheckLocale(alternate.Code),
				),
			},
			{
				PreConfig: func() {
					testAccSetLocale(t, original.Code)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccLocaleResourceConfig(alternate.Code),
				Check:  testAccCheckLocale(alternate.Code),
			},
			{
				Config: testAccLocaleResourceConfig(original.Code),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLocaleResource(
						original,
						original.Code,
					),
					testAccCheckLocale(original.Code),
				),
			},
			{
				Config: testAccLocaleResourceConfig(alternate.Code),
				Check:  testAccCheckLocale(alternate.Code),
			},
		},
	})
}

func TestAccLocaleResourceImport(t *testing.T) {
	original, alternate := testAccLocalePair(t)

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckLocale(original.Code),
		Steps: []testresource.TestStep{
			{
				Config: testAccLocaleResourceConfig(original.Code),
				Check: testAccCheckLocaleResource(
					original,
					original.Code,
				),
			},
			{
				ResourceName:                         "cpanel_locale.test",
				ImportStateId:                        cpanellocale.AccountIdentity,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "account",
			},
			{
				Config: testAccLocaleResourceConfig(alternate.Code),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckLocaleResource(
						alternate,
						original.Code,
					),
					testAccCheckLocale(alternate.Code),
				),
			},
		},
	})
}

func testAccLocaleResourceConfig(code string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_locale" "test" {
  locale = %q
}
`, code)
}

func testAccCheckLocaleResource(
	expected cpanellocale.Locale,
	restoreCode string,
) testresource.TestCheckFunc {
	const resourceName = "cpanel_locale.test"

	return testresource.ComposeAggregateTestCheckFunc(
		testresource.TestCheckResourceAttr(
			resourceName,
			"account",
			cpanellocale.AccountIdentity,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"locale",
			expected.Code,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"restore_locale",
			restoreCode,
		),
		testresource.TestCheckResourceAttrSet(resourceName, "name"),
		testresource.TestCheckResourceAttr(
			resourceName,
			"local_name",
			expected.LocalName,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"direction",
			expected.Direction,
		),
		testresource.TestCheckResourceAttr(
			resourceName,
			"encoding",
			"utf-8",
		),
	)
}

func testAccLocalePair(
	t *testing.T,
) (cpanellocale.Locale, cpanellocale.Locale) {
	t.Helper()

	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	client := testAccLocaleClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	current, err := client.GetCurrent(ctx)
	if err != nil {
		t.Fatalf("read current cPanel locale: %v", err)
	}
	locales, err := client.List(ctx)
	if err != nil {
		t.Fatalf("list cPanel locales: %v", err)
	}

	var alternate *cpanellocale.Locale
	for _, preferredCode := range []string{"en", "fr"} {
		if preferredCode == current.Code {
			continue
		}
		for index := range locales {
			if locales[index].Code == preferredCode {
				value := locales[index]
				alternate = &value
				break
			}
		}
		if alternate != nil {
			break
		}
	}
	if alternate == nil {
		for index := range locales {
			if locales[index].Code != current.Code {
				value := locales[index]
				alternate = &value
				break
			}
		}
	}
	if alternate == nil {
		t.Fatal("cPanel account does not expose an alternate locale")
	}

	original := *current
	t.Cleanup(func() {
		cleanupContext, cleanupCancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cleanupCancel()

		actual, cleanupErr := client.GetCurrent(cleanupContext)
		if cleanupErr != nil {
			t.Errorf("read cPanel locale during cleanup: %v", cleanupErr)
			return
		}
		if actual.Code == original.Code {
			return
		}
		if _, cleanupErr = client.Set(
			cleanupContext,
			original.Code,
		); cleanupErr != nil {
			t.Errorf(
				"restore cPanel locale %q during cleanup: %v",
				original.Code,
				cleanupErr,
			)
		}
	})

	return original, *alternate
}

func testAccLocaleClient(t *testing.T) *cpanellocale.Client {
	t.Helper()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	return cpanellocale.NewClient(client)
}

func testAccSetLocale(t *testing.T, code string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	if _, err := testAccLocaleClient(t).Set(ctx, code); err != nil {
		t.Fatalf("set cPanel locale %q: %v", code, err)
	}
}

func testAccCheckLocale(code string) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			90*time.Second,
		)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		current, err := cpanellocale.NewClient(client).GetCurrent(ctx)
		if err != nil {
			return err
		}
		if current.Code != code {
			return fmt.Errorf(
				"cPanel locale = %q, want %q",
				current.Code,
				code,
			)
		}

		return nil
	}
}
