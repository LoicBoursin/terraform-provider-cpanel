package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDomainAliasResource(t *testing.T) {
	const resourceName = "cpanel_domain_alias.test"

	firstDomain := testAccDomainAlias(t, "resource")
	secondDomain := testAccDomainAlias(t, "replacement")
	targetDomain := testAccMainDomain(t)
	const documentRoot = "public_html"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckDomainAliasesDestroyed(
			firstDomain,
			secondDomain,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccDomainAliasResourceConfig(firstDomain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", firstDomain),
					resource.TestCheckResourceAttr(
						resourceName,
						"target_domain",
						targetDomain,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"document_root",
						documentRoot,
					),
					testAccCheckDomainAliasExists(
						firstDomain,
						targetDomain,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstDomain,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "domain",
			},
			{
				Config: testAccDomainAliasResourceConfig(secondDomain),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "domain", secondDomain),
					testAccCheckDomainAliasExists(
						secondDomain,
						targetDomain,
					),
					testAccCheckDomainAliasesDestroyed(firstDomain),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteDomainAlias(t, secondDomain)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccDomainAliasResourceConfig(secondDomain),
				Check: testAccCheckDomainAliasExists(
					secondDomain,
					targetDomain,
				),
			},
		},
	})
}

func testAccDomainAliasResourceConfig(domain string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_domain_alias" "test" {
  domain = %q
}
`, domain)
}
