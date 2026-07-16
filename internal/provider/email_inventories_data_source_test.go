package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailInventoryDataSourceSchemas(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		constructor func() datasource.DataSource
		attribute   string
		fields      []string
		forbidden   []string
	}{
		{
			name:        "accounts",
			constructor: NewEmailAccountsDataSource,
			attribute:   "addresses",
			forbidden: []string{
				"quota",
				"disk_used",
				"suspended",
				"user",
				"domain",
			},
		},
		{
			name:        "domains",
			constructor: NewEmailDomainsDataSource,
			attribute:   "domains",
		},
		{
			name:        "routings",
			constructor: NewEmailRoutingsDataSource,
			attribute:   "routings",
			fields:      []string{"domain", "mode"},
			forbidden: []string{
				"detected",
				"entries",
				"mx",
			},
		},
		{
			name:        "domain forwarders",
			constructor: NewEmailDomainForwardersDataSource,
			attribute:   "forwarders",
			fields:      []string{"destination", "domain"},
		},
		{
			name:        "mailing lists",
			constructor: NewEmailMailingListsDataSource,
			attribute:   "addresses",
			forbidden: []string{
				"administrator",
				"disk_used",
				"privacy",
			},
		},
		{
			name:        "autoresponders",
			constructor: NewEmailAutoRespondersDataSource,
			attribute:   "addresses",
			forbidden: []string{
				"body",
				"charset",
				"from",
				"interval",
				"start",
				"stop",
				"subject",
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := &datasource.SchemaResponse{}
			test.constructor().Schema(
				t.Context(),
				datasource.SchemaRequest{},
				response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf(
					"Schema() diagnostics: %v",
					response.Diagnostics,
				)
			}
			if len(response.Schema.Attributes) != 1 {
				t.Fatalf(
					"top-level attribute count = %d, want 1",
					len(response.Schema.Attributes),
				)
			}
			if len(test.fields) == 0 {
				attribute, ok := response.Schema.Attributes[test.attribute].(datasourceschema.ListAttribute)
				if !ok || !attribute.Computed {
					t.Fatalf(
						"%s must be a computed string list",
						test.attribute,
					)
				}
				if attribute.ElementType != types.StringType {
					t.Fatalf(
						"%s element type = %T, want string",
						test.attribute,
						attribute.ElementType,
					)
				}
				for _, forbidden := range test.forbidden {
					if _, exists := response.Schema.Attributes[forbidden]; exists {
						t.Fatalf(
							"schema must not expose %q",
							forbidden,
						)
					}
				}
				return
			}

			attribute, ok := response.Schema.Attributes[test.attribute].(datasourceschema.ListNestedAttribute)
			if !ok || !attribute.Computed {
				t.Fatalf(
					"%s must be a computed nested list",
					test.attribute,
				)
			}
			if len(attribute.NestedObject.Attributes) != len(test.fields) {
				t.Fatalf(
					"%s nested field count = %d, want %d",
					test.attribute,
					len(attribute.NestedObject.Attributes),
					len(test.fields),
				)
			}
			for _, field := range test.fields {
				if _, exists := attribute.NestedObject.Attributes[field]; !exists {
					t.Fatalf(
						"%s must expose %q",
						test.attribute,
						field,
					)
				}
			}
			for _, forbidden := range test.forbidden {
				if _, exists := attribute.NestedObject.Attributes[forbidden]; exists {
					t.Fatalf(
						"%s must not expose %q",
						test.attribute,
						forbidden,
					)
				}
			}
		})
	}
}

func TestAccEmailInventoryDataSources(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadEmailInventories(t)
	t.Cleanup(func() {
		testAccRequireEmailInventories(t, baseline)
	})
	accountAddress := testAccEmailAddress(t, "inventory")
	mailingListAddress := testAccEmailMailingListAddress(t, "inventory")
	autoResponderAddress := testAccEmailAutoResponderAddress(t, "inventory")
	forwarderDomain := testAccMainDomain(t)
	forwarderDestination := testAccEmailDomainForwarderDestination(
		"inventory",
	)
	password := "Iv9!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)

	const (
		accountsName         = "data.cpanel_email_accounts.all"
		domainsName          = "data.cpanel_email_domains.all"
		routingsName         = "data.cpanel_email_routings.all"
		domainForwardersName = "data.cpanel_email_domain_forwarders.all"
		mailingListsName     = "data.cpanel_email_mailing_lists.all"
		autoRespondersName   = "data.cpanel_email_auto_responders.all"
	)
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			accountsName,
			"addresses.#",
			strconv.Itoa(len(baseline.Accounts)),
		),
		testresource.TestCheckResourceAttr(
			domainsName,
			"domains.#",
			strconv.Itoa(len(baseline.Domains)),
		),
		testresource.TestCheckResourceAttr(
			routingsName,
			"routings.#",
			strconv.Itoa(len(baseline.Routings)),
		),
		testresource.TestCheckResourceAttr(
			domainForwardersName,
			"forwarders.#",
			strconv.Itoa(len(baseline.DomainForwarders)),
		),
		testresource.TestCheckResourceAttr(
			mailingListsName,
			"addresses.#",
			strconv.Itoa(len(baseline.MailingLists)),
		),
		testresource.TestCheckResourceAttr(
			autoRespondersName,
			"addresses.#",
			strconv.Itoa(len(baseline.AutoResponders)),
		),
	}
	checks = appendEmailInventoryStringChecks(
		checks,
		accountsName,
		"addresses",
		baseline.Accounts,
	)
	checks = appendEmailInventoryStringChecks(
		checks,
		domainsName,
		"domains",
		baseline.Domains,
	)
	checks = appendEmailInventoryStringChecks(
		checks,
		mailingListsName,
		"addresses",
		baseline.MailingLists,
	)
	checks = appendEmailInventoryStringChecks(
		checks,
		autoRespondersName,
		"addresses",
		baseline.AutoResponders,
	)
	for index, routing := range baseline.Routings {
		prefix := fmt.Sprintf("routings.%d.", index)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				routingsName,
				prefix+"domain",
				routing.Domain,
			),
			testresource.TestCheckResourceAttr(
				routingsName,
				prefix+"mode",
				string(routing.Mode),
			),
		)
	}
	for index, forwarder := range baseline.DomainForwarders {
		prefix := fmt.Sprintf("forwarders.%d.", index)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				domainForwardersName,
				prefix+"domain",
				forwarder.Domain,
			),
			testresource.TestCheckResourceAttr(
				domainForwardersName,
				prefix+"destination",
				forwarder.Destination,
			),
		)
	}

	fixtureChecks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			accountsName,
			"addresses.#",
			strconv.Itoa(len(baseline.Accounts)+1),
		),
		testresource.TestCheckTypeSetElemAttr(
			accountsName,
			"addresses.*",
			accountAddress,
		),
		testresource.TestCheckResourceAttr(
			domainsName,
			"domains.#",
			strconv.Itoa(len(baseline.Domains)),
		),
		testresource.TestCheckResourceAttr(
			routingsName,
			"routings.#",
			strconv.Itoa(len(baseline.Routings)),
		),
		testresource.TestCheckResourceAttr(
			domainForwardersName,
			"forwarders.#",
			strconv.Itoa(len(baseline.DomainForwarders)+1),
		),
		testresource.TestCheckTypeSetElemNestedAttrs(
			domainForwardersName,
			"forwarders.*",
			map[string]string{
				"destination": forwarderDestination,
				"domain":      forwarderDomain,
			},
		),
		testresource.TestCheckResourceAttr(
			mailingListsName,
			"addresses.#",
			strconv.Itoa(len(baseline.MailingLists)+1),
		),
		testresource.TestCheckTypeSetElemAttr(
			mailingListsName,
			"addresses.*",
			mailingListAddress,
		),
		testresource.TestCheckResourceAttr(
			autoRespondersName,
			"addresses.#",
			strconv.Itoa(len(baseline.AutoResponders)+1),
		),
		testresource.TestCheckTypeSetElemAttr(
			autoRespondersName,
			"addresses.*",
			autoResponderAddress,
		),
	}
	config := testAccEmailInventoryDataSourcesConfig()
	fixtureConfig := testAccEmailInventoryFixtureConfig(
		accountAddress,
		mailingListAddress,
		autoResponderAddress,
		forwarderDomain,
		forwarderDestination,
		password,
	)
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireEmailInventories(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeTestCheckFunc(
			testAccCheckEmailAccountsDestroyed(accountAddress),
			testAccCheckEmailMailingListsDestroyed(mailingListAddress),
			testAccCheckEmailAutoRespondersDestroyed(autoResponderAddress),
			testAccCheckEmailDomainForwarderDestroyed(forwarderDomain),
		),
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					checks...,
				),
			},
			{
				Config: config,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: fixtureConfig,
				Check: testresource.ComposeAggregateTestCheckFunc(
					fixtureChecks...,
				),
			},
			{
				Config: fixtureConfig,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
			{
				Config: `provider "cpanel" {}`,
			},
			{
				PreConfig: func() {
					testAccRequireEmailInventories(t, baseline)
				},
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					checks...,
				),
			},
			{
				Config: config,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccEmailInventoryDataSourcesConfig() string {
	return providerConfig + `
data "cpanel_email_accounts" "all" {}

data "cpanel_email_domains" "all" {}

data "cpanel_email_routings" "all" {}

data "cpanel_email_domain_forwarders" "all" {}

data "cpanel_email_mailing_lists" "all" {}

data "cpanel_email_auto_responders" "all" {}
`
}

func testAccEmailInventoryFixtureConfig(
	accountAddress string,
	mailingListAddress string,
	autoResponderAddress string,
	forwarderDomain string,
	forwarderDestination string,
	password string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "inventory" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 50
  delete_on_destroy = true
}

resource "cpanel_email_mailing_list" "inventory" {
  address          = %q
  password         = %q
  password_version = 1
  delete_on_destroy = true
  advertised       = false
  archive_private  = true
  subscribe_policy = 3
}

resource "cpanel_email_auto_responder" "inventory" {
  email          = %q
  from           = "Terraform inventory acceptance"
  subject        = "Inventory acceptance"
  body           = "Temporary autoresponder for inventory acceptance."
  charset        = "UTF-8"
  interval_hours = 8
  is_html        = false
  start_unix     = 0
  stop_unix      = 0
}

resource "cpanel_email_domain_forwarder" "inventory" {
  domain      = %q
  destination = %q
}

data "cpanel_email_accounts" "all" {
  depends_on = [cpanel_email_account.inventory]
}

data "cpanel_email_domains" "all" {}

data "cpanel_email_routings" "all" {}

data "cpanel_email_domain_forwarders" "all" {
  depends_on = [cpanel_email_domain_forwarder.inventory]
}

data "cpanel_email_mailing_lists" "all" {
  depends_on = [cpanel_email_mailing_list.inventory]
}

data "cpanel_email_auto_responders" "all" {
  depends_on = [cpanel_email_auto_responder.inventory]
}
`,
		accountAddress,
		password,
		mailingListAddress,
		password,
		autoResponderAddress,
		forwarderDomain,
		forwarderDestination,
	)
}

type testAccEmailInventoryBaseline struct {
	Accounts         []string
	Domains          []string
	Routings         []cpanelmail.RoutingDefinition
	DomainForwarders []cpanelmail.DomainForwarder
	MailingLists     []string
	AutoResponders   []string
}

func testAccReadEmailInventories(
	t *testing.T,
) testAccEmailInventoryBaseline {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)

	accounts, err := emailClient.ListAccountAddresses(ctx)
	if err != nil {
		t.Fatalf("read cPanel email account inventory: %v", err)
	}
	domains, err := emailClient.ListMailDomainInventory(ctx)
	if err != nil {
		t.Fatalf("read cPanel mail domain inventory: %v", err)
	}
	routings, err := emailClient.ListRoutingDefinitions(ctx)
	if err != nil {
		t.Fatalf("read cPanel email routing inventory: %v", err)
	}
	domainForwarders, err := emailClient.ListDomainForwarders(ctx)
	if err != nil {
		t.Fatalf(
			"read cPanel email domain forwarder inventory: %v",
			err,
		)
	}
	mailingLists, err := emailClient.ListMailingListAddresses(ctx)
	if err != nil {
		t.Fatalf("read cPanel mailing list inventory: %v", err)
	}
	autoResponders, err := emailClient.ListAutoResponderAddresses(ctx)
	if err != nil {
		t.Fatalf("read cPanel autoresponder inventory: %v", err)
	}

	return testAccEmailInventoryBaseline{
		Accounts:         accounts,
		Domains:          domains,
		Routings:         routings,
		DomainForwarders: domainForwarders,
		MailingLists:     mailingLists,
		AutoResponders:   autoResponders,
	}
}

func testAccRequireEmailInventories(
	t *testing.T,
	expected testAccEmailInventoryBaseline,
) {
	t.Helper()

	actual := testAccReadEmailInventories(t)
	if reflect.DeepEqual(actual, expected) {
		return
	}
	t.Fatalf(
		"cPanel email inventories changed: "+
			"accounts %d/%d, domains %d/%d, routings %d/%d, "+
			"domain forwarders %d/%d, mailing lists %d/%d, "+
			"autoresponders %d/%d",
		len(actual.Accounts),
		len(expected.Accounts),
		len(actual.Domains),
		len(expected.Domains),
		len(actual.Routings),
		len(expected.Routings),
		len(actual.DomainForwarders),
		len(expected.DomainForwarders),
		len(actual.MailingLists),
		len(expected.MailingLists),
		len(actual.AutoResponders),
		len(expected.AutoResponders),
	)
}

func appendEmailInventoryStringChecks(
	checks []testresource.TestCheckFunc,
	resourceName string,
	attribute string,
	values []string,
) []testresource.TestCheckFunc {
	for index, value := range values {
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				resourceName,
				fmt.Sprintf("%s.%d", attribute, index),
				value,
			),
		)
	}

	return checks
}
