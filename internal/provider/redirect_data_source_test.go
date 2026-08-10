package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

func TestAccRedirectDataSource(t *testing.T) {
	domain := testAccMainDomain(t)
	source := testAccRedirectSource("datasource")
	destination := testAccRedirectDestination("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckRedirectsDestroyed(domain, source),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_redirect" "fixture" {
  domain      = %q
  source      = %q
  destination = %q
  type        = "temporary"
  www_mode    = "without"
  wildcard    = true
}

data "cpanel_redirect" "test" {
  domain = cpanel_redirect.fixture.domain
  source = cpanel_redirect.fixture.source
}
`, domain, source, destination),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"source",
						source,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"destination",
						destination,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"type",
						cpanelredirect.TypeTemporary,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"www_mode",
						cpanelredirect.WWWModeWithout,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"wildcard",
						"true",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"status_code",
						"302",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_redirect.test",
						"document_root",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_redirect.test",
						"kind",
						"rewrite",
					),
					testAccCheckRedirectExists(cpanelredirect.Definition{
						Domain:      domain,
						Source:      source,
						Destination: destination,
						Type:        cpanelredirect.TypeTemporary,
						WWWMode:     cpanelredirect.WWWModeWithout,
						Wildcard:    true,
					}),
				),
			},
		},
	})
}
