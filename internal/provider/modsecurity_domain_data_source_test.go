package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel/modsecurity"
)

func TestAccModSecurityDomainDataSource(t *testing.T) {
	domain := testAccSubdomain(t, "modsecuritydatasource")
	documentRoot := testAccDomainDocumentRoot("modsecurity-datasource")

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccModSecurityPreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: resource.ComposeTestCheckFunc(
			testAccCheckModSecurityDomainsDestroyed(domain),
			testAccCheckSubdomainsDestroyed(domain),
		),
		Steps: []resource.TestStep{
			{
				Config: testAccModSecurityDataSourceConfig(
					domain,
					documentRoot,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_modsecurity_domain.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_modsecurity_domain.test",
						"enabled",
						"true",
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_modsecurity_domain.test",
						"domain_type",
						modsecurity.DomainTypeSub,
					),
					resource.TestCheckTypeSetElemAttr(
						"data.cpanel_modsecurity_domain.test",
						"affected_domains.*",
						domain,
					),
					testAccCheckModSecurityDomainIsolation(domain),
					testAccCheckModSecurityDomain(domain, true),
				),
			},
		},
	})
}

func testAccModSecurityDataSourceConfig(
	domain string,
	documentRoot string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_subdomain" "fixture" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}

data "cpanel_modsecurity_domain" "test" {
  domain = cpanel_subdomain.fixture.domain
}
`, domain, documentRoot)
}
