package provider

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestAccAccountEmailFilterResource(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	const resourceName = "cpanel_account_email_filter.test"

	account := os.Getenv("CPANEL_USERNAME")
	first := cpanelmail.Filter{
		Account: account,
		Name:    testAccEmailFilterName("accountfirst"),
		Enabled: true,
		Rules: []cpanelmail.FilterRule{
			{
				Part:     "$header_subject:",
				Match:    "contains",
				Value:    "tfcpanel-account-filter-first",
				Operator: "and",
			},
			{
				Part:  "$header_from:",
				Match: "ends",
				Value: "@example.net",
			},
		},
		Actions: []cpanelmail.FilterAction{
			{
				Action:      "fail",
				Destination: "Terraform account filter rejection",
			},
			{Action: "finish"},
		},
	}
	second := cpanelmail.Filter{
		Account: account,
		Name:    testAccEmailFilterName("accountsecond"),
		Enabled: false,
		Rules: []cpanelmail.FilterRule{{
			Part:  "$message_body",
			Match: "contains",
			Value: "tfcpanel-account-filter-second",
		}},
		Actions: []cpanelmail.FilterAction{{Action: "finish"}},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckAccountEmailFiltersDestroyed(first.Name, second.Name),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccAccountEmailFilterConfig(first, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"account",
						account,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"name",
						first.Name,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"enabled",
						"true",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"rules.#",
						"2",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"actions.#",
						"2",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_account_email_filter.test",
						"account",
						account,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_account_email_filter.test",
						"name",
						first.Name,
					),
					testAccCheckAccountEmailFilterExists(first),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        first.Name,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
			},
			{
				Config: testAccAccountEmailFilterConfig(second, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"name",
						second.Name,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"enabled",
						"false",
					),
					testAccCheckAccountEmailFilterExists(second),
					testAccCheckAccountEmailFiltersDestroyed(first.Name),
				),
			},
			{
				PreConfig: func() {
					testAccSetAccountEmailFilterEnabled(
						t,
						second.Name,
						true,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				Check: resource.TestCheckResourceAttr(
					resourceName,
					"enabled",
					"true",
				),
			},
			{
				Config: testAccAccountEmailFilterConfig(second, false),
				Check:  testAccCheckAccountEmailFilterExists(second),
			},
			{
				PreConfig: func() {
					testAccDeleteAccountEmailFilter(t, second.Name)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccAccountEmailFilterConfig(second, false),
				Check:  testAccCheckAccountEmailFilterExists(second),
			},
		},
	})
}

func testAccAccountEmailFilterConfig(
	filter cpanelmail.Filter,
	includeDataSource bool,
) string {
	var builder strings.Builder
	builder.WriteString(providerConfig)
	writeTestAccAccountEmailFilterResource(&builder, "test", filter)
	if includeDataSource {
		builder.WriteString(`
data "cpanel_account_email_filter" "test" {
  name = cpanel_account_email_filter.test.name
}
`)
	}

	return builder.String()
}

func writeTestAccAccountEmailFilterResource(
	builder *strings.Builder,
	resourceLabel string,
	filter cpanelmail.Filter,
) {
	fmt.Fprintf(
		builder,
		`
resource "cpanel_account_email_filter" %q {
  name    = %q
  enabled = %t

  rules = [
`,
		resourceLabel,
		filter.Name,
		filter.Enabled,
	)
	for _, rule := range filter.Rules {
		match := rule.Match
		if match == "" {
			match = "none"
		}
		operator := rule.Operator
		if operator == "" {
			operator = "none"
		}
		fmt.Fprintf(
			builder,
			`    {
      part     = %q
      match    = %q
      value    = %q
      operator = %q
    },
`,
			rule.Part,
			match,
			rule.Value,
			operator,
		)
	}
	builder.WriteString("  ]\n\n  actions = [\n")
	for _, action := range filter.Actions {
		fmt.Fprintf(
			builder,
			`    {
      action      = %q
`,
			action.Action,
		)
		if action.Destination == "" {
			builder.WriteString("      destination = null\n")
		} else {
			fmt.Fprintf(
				builder,
				"      destination = %q\n",
				action.Destination,
			)
		}
		builder.WriteString("    },\n")
	}
	builder.WriteString("  ]\n}\n")
}

func testAccCheckAccountEmailFilterExists(
	expected cpanelmail.Filter,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filter, err := cpanelmail.NewClient(client).GetAccountFilter(
			ctx,
			expected.Name,
		)
		if err != nil {
			return err
		}
		if filter == nil {
			return fmt.Errorf(
				"account email filter %q was not found",
				expected.Name,
			)
		}
		if !emailFiltersEqual(*filter, expected) {
			return fmt.Errorf(
				"account email filter = %#v, want %#v",
				*filter,
				expected,
			)
		}

		return nil
	}
}

func testAccCheckAccountEmailFiltersDestroyed(
	names ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filters, err := cpanelmail.NewClient(client).ListAccountFilters(ctx)
		if err != nil {
			return err
		}
		for _, filter := range filters {
			for _, name := range names {
				if filter.Name == name {
					return fmt.Errorf(
						"account email filter %q still exists",
						name,
					)
				}
			}
		}

		return nil
	}
}

func testAccSetAccountEmailFilterEnabled(
	t *testing.T,
	name string,
	enabled bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).SetAccountFilterEnabled(
		ctx,
		name,
		enabled,
	); err != nil {
		t.Fatalf(
			"set account email filter %q enabled=%t: %v",
			name,
			enabled,
			err,
		)
	}
}

func testAccDeleteAccountEmailFilter(t *testing.T, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteAccountFilter(
		ctx,
		name,
	); err != nil {
		t.Fatalf("delete account email filter %q: %v", name, err)
	}
}
