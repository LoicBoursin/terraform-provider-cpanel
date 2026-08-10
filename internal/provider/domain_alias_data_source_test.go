package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDomainAliasDataSource(t *testing.T) {
	domain := testAccDomainAlias(t, "datasource")
	targetDomain := testAccMainDomain(t)
	const documentRoot = "public_html"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckDomainAliasesDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: testAccDomainAliasDataSourceConfig(domain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_domain_alias.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_domain_alias.test",
						"target_domain",
						targetDomain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_domain_alias.test",
						"document_root",
						documentRoot,
					),
					testAccCheckDomainAliasExists(
						domain,
						targetDomain,
					),
				),
			},
		},
	})
}

func testAccDomainAliasDataSourceConfig(domain string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_domain_alias" "fixture" {
  domain = %q
}

data "cpanel_domain_alias" "test" {
  domain = cpanel_domain_alias.fixture.domain
}
`, domain)
}
