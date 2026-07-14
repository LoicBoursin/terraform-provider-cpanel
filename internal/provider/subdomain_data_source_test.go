package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccSubdomainDataSource(t *testing.T) {
	domain := testAccSubdomain(t, "datasource")
	documentRoot := testAccDomainDocumentRoot("sub-datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckSubdomainsDestroyed(domain),
		Steps: []resource.TestStep{
			{
				Config: testAccSubdomainDataSourceConfig(domain, documentRoot),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_subdomain.test",
						"domain",
						domain,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_subdomain.test",
						"document_root",
						documentRoot,
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_subdomain.test",
						"subdomain",
					),
					resource.TestCheckResourceAttrSet(
						"data.cpanel_subdomain.test",
						"root_domain",
					),
					testAccCheckSubdomainExists(domain, documentRoot),
				),
			},
		},
	})
}

func testAccSubdomainDataSourceConfig(domain, documentRoot string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_subdomain" "fixture" {
  domain               = %q
  document_root        = %q
  delete_document_root = false
}

data "cpanel_subdomain" "test" {
  domain = cpanel_subdomain.fixture.domain
}
`, domain, documentRoot)
}
