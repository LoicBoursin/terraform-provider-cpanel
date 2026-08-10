package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestAccEmailFilterResource(t *testing.T) {
	const (
		resourceName = "cpanel_email_filter.test"
		password     = "N8!emailFilterFixture-2026"
	)

	address := testAccEmailFilterAddress(t, "resource")
	first := cpanelmail.Filter{
		Account: address,
		Name:    testAccEmailFilterName("first"),
		Enabled: true,
		Rules: []cpanelmail.FilterRule{
			{
				Part:     "$header_subject:",
				Match:    "contains",
				Value:    "tfcpanel-filter-first",
				Operator: "and",
			},
			{
				Part:     "$h_x-Spam-Score:",
				Match:    "is above",
				Value:    "5",
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
				Action:      "deliver",
				Destination: "tfcpanelfilter@example.net",
			},
			{
				Action:      "fail",
				Destination: "Terraform managed rejection",
			},
			{Action: "finish"},
		},
	}
	second := cpanelmail.Filter{
		Account: address,
		Name:    testAccEmailFilterName("second"),
		Enabled: false,
		Rules: []cpanelmail.FilterRule{
			{
				Part:  "$header_subject:",
				Match: "is",
				Value: "tfcpanel-filter-second",
			},
		},
		Actions: []cpanelmail.FilterAction{{Action: "finish"}},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckEmailFiltersDestroyed(
				address,
				first.Name,
				second.Name,
			),
			testAccCheckEmailAccountsDestroyed(address),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailFilterResourceConfig(first, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"account",
						address,
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
						"3",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"rules.0.operator",
						"and",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"rules.1.operator",
						"and",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"rules.2.operator",
						"none",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"actions.#",
						"3",
					),
					testAccCheckEmailFilterExists(first),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        address + "|" + first.Name,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "account",
			},
			{
				Config: testAccEmailFilterResourceConfig(second, password),
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
					resource.TestCheckResourceAttr(
						resourceName,
						"rules.#",
						"1",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"actions.0.action",
						"finish",
					),
					testAccCheckEmailFilterExists(second),
					testAccCheckEmailFiltersDestroyed(address, first.Name),
				),
			},
			{
				PreConfig: func() {
					testAccSetEmailFilterEnabled(
						t,
						address,
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
				Config: testAccEmailFilterResourceConfig(second, password),
				Check:  testAccCheckEmailFilterExists(second),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailFilter(t, address, second.Name)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailFilterResourceConfig(second, password),
				Check:  testAccCheckEmailFilterExists(second),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailAccount(t, second.Account)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailFilterResourceConfig(second, password),
				Check:  testAccCheckEmailFilterExists(second),
			},
		},
	})
}

func TestAccEmailFilterResourcesSerializeSameMailbox(t *testing.T) {
	const password = "S7!emailFilterConcurrency-2026"

	address := testAccEmailFilterAddress(t, "concurrency")
	first := cpanelmail.Filter{
		Account: address,
		Name:    testAccEmailFilterName("concurrentfirst"),
		Enabled: true,
		Rules: []cpanelmail.FilterRule{{
			Part:  "$header_subject:",
			Match: "contains",
			Value: "tfcpanel-concurrent-first",
		}},
		Actions: []cpanelmail.FilterAction{{Action: "finish"}},
	}
	second := cpanelmail.Filter{
		Account: address,
		Name:    testAccEmailFilterName("concurrentsecond"),
		Enabled: false,
		Rules: []cpanelmail.FilterRule{{
			Part:  "$message_body",
			Match: "contains",
			Value: "tfcpanel-concurrent-second",
		}},
		Actions: []cpanelmail.FilterAction{{
			Action:      "fail",
			Destination: "Terraform concurrent rejection",
		}},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckEmailFiltersDestroyed(
				address,
				first.Name,
				second.Name,
			),
			testAccCheckEmailAccountsDestroyed(address),
		),
		Steps: []resource.TestStep{{
			Config: testAccEmailFilterPairConfig(first, second, password),
			Check: resource.ComposeAggregateTestCheckFunc(
				testAccCheckEmailFilterExists(first),
				testAccCheckEmailFilterExists(second),
			),
		}},
	})
}

func TestAccEmailFilterResourceSupportsCanonicalParts(t *testing.T) {
	const password = "T5!emailFilterParts-2026"

	filter := cpanelmail.Filter{
		Account: testAccEmailFilterAddress(t, "parts"),
		Name:    testAccEmailFilterName("parts"),
		Enabled: true,
		Actions: []cpanelmail.FilterAction{
			{
				Action:      "fail",
				Destination: "null",
			},
			{Action: "finish"},
		},
	}
	for index, part := range emailFilterParts {
		match := "contains"
		value := fmt.Sprintf("tfcpanel-part-%d", index)
		if part == "$h_x-Spam-Score:" {
			match = "is above"
			value = "5"
		} else if cpanelmail.IsMatchlessFilterPart(part) {
			match = ""
			value = ""
		}

		operator := "and"
		if index == len(emailFilterParts)-1 {
			operator = ""
		}
		filter.Rules = append(filter.Rules, cpanelmail.FilterRule{
			Part:     part,
			Match:    match,
			Value:    value,
			Operator: operator,
		})
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckEmailFiltersDestroyed(filter.Account, filter.Name),
			testAccCheckEmailAccountsDestroyed(filter.Account),
		),
		Steps: []resource.TestStep{{
			Config: testAccEmailFilterResourceConfig(filter, password),
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr(
					"cpanel_email_filter.test",
					"rules.#",
					fmt.Sprintf("%d", len(emailFilterParts)),
				),
				resource.TestCheckResourceAttr(
					"cpanel_email_filter.test",
					"actions.0.destination",
					"null",
				),
				testAccCheckEmailFilterExists(filter),
			),
		}},
	})
}

func TestAccEmailFilterResourceRejectsExistingFilter(t *testing.T) {
	const password = "C8!emailFilterCollision-2026"

	filter := cpanelmail.Filter{
		Account: testAccEmailFilterAddress(t, "collision"),
		Name:    testAccEmailFilterName("collision"),
		Enabled: true,
		Rules: []cpanelmail.FilterRule{{
			Part:  "$header_subject:",
			Match: "contains",
			Value: "tfcpanel-collision",
		}},
		Actions: []cpanelmail.FilterAction{{Action: "finish"}},
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeAggregateTestCheckFunc(
			testAccCheckEmailFiltersDestroyed(filter.Account, filter.Name),
			testAccCheckEmailAccountsDestroyed(filter.Account),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailFilterAccountConfig(
					filter.Account,
					password,
				),
			},
			{
				PreConfig: func() {
					testAccStoreEmailFilter(t, filter)
				},
				Config:      testAccEmailFilterResourceConfig(filter, password),
				ExpectError: regexp.MustCompile("Email filter already exists"),
			},
		},
	})
}

func testAccEmailFilterResourceConfig(
	filter cpanelmail.Filter,
	password string,
) string {
	var builder strings.Builder
	builder.WriteString(testAccEmailFilterAccountConfig(filter.Account, password))
	writeTestAccEmailFilterResource(&builder, "test", filter)

	return builder.String()
}

func testAccEmailFilterAccountConfig(address, password string) string {
	return fmt.Sprintf(
		`%s
resource "cpanel_email_account" "filter_fixture" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 10
  delete_on_destroy = true
}
`,
		providerConfig,
		address,
		password,
	)
}

func writeTestAccEmailFilterResource(
	builder *strings.Builder,
	resourceLabel string,
	filter cpanelmail.Filter,
) {
	fmt.Fprintf(
		builder,
		`
resource "cpanel_email_filter" %q {
  account = cpanel_email_account.filter_fixture.email
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

func testAccEmailFilterPairConfig(
	first cpanelmail.Filter,
	second cpanelmail.Filter,
	password string,
) string {
	var builder strings.Builder
	builder.WriteString(testAccEmailFilterAccountConfig(first.Account, password))
	writeTestAccEmailFilterResource(&builder, "first", first)
	writeTestAccEmailFilterResource(&builder, "second", second)

	return builder.String()
}

func testAccCheckEmailFilterExists(
	expected cpanelmail.Filter,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filter, err := cpanelmail.NewClient(client).GetFilter(
			ctx,
			expected.Account,
			expected.Name,
		)
		if err != nil {
			return err
		}
		if filter == nil {
			return fmt.Errorf(
				"email filter %q for %q was not found",
				expected.Name,
				expected.Account,
			)
		}
		if !emailFiltersEqual(*filter, expected) {
			return fmt.Errorf(
				"email filter = %#v, want %#v",
				*filter,
				expected,
			)
		}

		return nil
	}
}

func testAccCheckEmailFiltersDestroyed(
	account string,
	names ...string,
) resource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		filters, err := cpanelmail.NewClient(client).ListFilters(ctx, account)
		if err != nil {
			return err
		}
		for _, filter := range filters {
			for _, name := range names {
				if filter.Name == name {
					return fmt.Errorf(
						"email filter %q for %q still exists",
						name,
						account,
					)
				}
			}
		}

		return nil
	}
}

func testAccSetEmailFilterEnabled(
	t *testing.T,
	account string,
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
	if err := cpanelmail.NewClient(client).SetFilterEnabled(
		ctx,
		account,
		name,
		enabled,
	); err != nil {
		t.Fatalf("set email filter %q enabled=%t: %v", name, enabled, err)
	}
}

func testAccDeleteEmailFilter(t *testing.T, account string, name string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelmail.NewClient(client).DeleteFilter(
		ctx,
		account,
		name,
	); err != nil {
		t.Fatalf("delete email filter %q for %q: %v", name, account, err)
	}
}

func testAccStoreEmailFilter(t *testing.T, filter cpanelmail.Filter) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)
	if err := emailClient.StoreFilter(
		ctx,
		filter.Account,
		"",
		filter,
	); err != nil {
		t.Fatalf(
			"store email filter %q for %q: %v",
			filter.Name,
			filter.Account,
			err,
		)
	}
	if err := emailClient.SetFilterEnabled(
		ctx,
		filter.Account,
		filter.Name,
		filter.Enabled,
	); err != nil {
		t.Fatalf(
			"set email filter %q enabled=%t: %v",
			filter.Name,
			filter.Enabled,
			err,
		)
	}
	actual, err := emailClient.GetFilter(ctx, filter.Account, filter.Name)
	if err != nil {
		t.Fatalf("read stored email filter %q: %v", filter.Name, err)
	}
	if actual == nil || !emailFiltersEqual(*actual, filter) {
		t.Fatalf("stored email filter = %#v, want %#v", actual, filter)
	}
}
