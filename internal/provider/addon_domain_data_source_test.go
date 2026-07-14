package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAddonDomainDataSource(t *testing.T) {
	domain, internalSubdomain := testAccAddonDomain(t, "datasource")
	documentRoot := testAccDomainDocumentRoot("addon-datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAddonDomainsDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: testAccAddonDomainDataSourceConfig(
					domain,
					internalSubdomain,
					documentRoot,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_addon_domain.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_addon_domain.test",
						"internal_subdomain",
						internalSubdomain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_addon_domain.test",
						"document_root",
						documentRoot,
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_addon_domain.test",
						"root_domain",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_addon_domain.test",
						"full_subdomain",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_addon_domain.test",
						"domain_key",
					),
					testAccCheckAddonDomainExists(
						domain,
						internalSubdomain,
						documentRoot,
					),
				),
			},
		},
	})
}

func testAccAddonDomainDataSourceConfig(
	domain string,
	internalSubdomain string,
	documentRoot string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_addon_domain" "fixture" {
  domain               = %q
  internal_subdomain   = %q
  document_root        = %q
  delete_document_root = false
}

data "cpanel_addon_domain" "test" {
  domain = cpanel_addon_domain.fixture.domain
}
`, domain, internalSubdomain, documentRoot)
}
