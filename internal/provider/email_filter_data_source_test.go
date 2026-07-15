package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestAccEmailFilterDataSource(t *testing.T) {
	const password = "P9!emailFilterDataSource-2026"

	filter := cpanelmail.Filter{
		Account: testAccEmailFilterAddress(t, "datasource"),
		Name:    testAccEmailFilterName("datasource"),
		Enabled: false,
		Rules: []cpanelmail.FilterRule{
			{
				Part:  "$message_body",
				Match: "contains",
				Value: "tfcpanel-data-source",
			},
		},
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
				Config: testAccEmailFilterDataSourceConfig(filter, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"account",
						filter.Account,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"name",
						filter.Name,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"enabled",
						"false",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"rules.0.part",
						"$message_body",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"rules.0.operator",
						"none",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.test",
						"actions.0.action",
						"finish",
					),
					testAccCheckEmailFilterExists(filter),
				),
			},
		},
	})
}

func TestAccEmailFilterDataSourceReadsExternalSaveAction(t *testing.T) {
	const password = "V4!emailFilterExternal-2026"

	filter := cpanelmail.Filter{
		Account: testAccEmailFilterAddress(t, "externaldatasource"),
		Name:    testAccEmailFilterName("externaldatasource"),
		Enabled: true,
		Rules: []cpanelmail.FilterRule{{
			Part:  "$message_body",
			Match: "contains",
			Value: "tfcpanel-external-data-source",
		}},
		Actions: []cpanelmail.FilterAction{{
			Action:      "save",
			Destination: "/.tfcpanelfiltersave",
		}},
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
				Config: testAccExternalEmailFilterDataSourceConfig(
					filter,
					password,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.external",
						"actions.0.action",
						"save",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_filter.external",
						"actions.0.destination",
						filter.Actions[0].Destination,
					),
				),
			},
		},
	})
}

func testAccEmailFilterDataSourceConfig(
	filter cpanelmail.Filter,
	password string,
) string {
	return testAccEmailFilterResourceConfig(filter, password) + `
data "cpanel_email_filter" "test" {
  account = cpanel_email_filter.test.account
  name    = cpanel_email_filter.test.name
}
`
}

func testAccExternalEmailFilterDataSourceConfig(
	filter cpanelmail.Filter,
	password string,
) string {
	return testAccEmailFilterAccountConfig(filter.Account, password) + `
data "cpanel_email_filter" "external" {
  account = cpanel_email_account.filter_fixture.email
  name    = ` + fmt.Sprintf("%q", filter.Name) + `
}
`
}
