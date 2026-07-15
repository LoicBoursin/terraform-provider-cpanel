package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

func TestAccLocaleDataSource(t *testing.T) {
	current, _ := testAccLocalePair(t)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + `
data "cpanel_locale" "current" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"account",
						cpanellocale.AccountIdentity,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"locale",
						current.Code,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"name",
						current.Name,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"local_name",
						current.LocalName,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"direction",
						current.Direction,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_locale.current",
						"encoding",
						current.Encoding,
					),
				),
			},
		},
	})
}
