package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelapachehandler "terraform-provider-cpanel/internal/cpanel/apachehandler"
)

func TestAccApacheHandlerDataSource(t *testing.T) {
	extension := testAccApacheHandlerExtension("datasource")
	handler := testAccApacheHandlerName("datasource")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckApacheHandlersDestroyed(extension),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_apache_handler" "fixture" {
  extension = %q
  handler   = %q
}

data "cpanel_apache_handler" "test" {
  extension = cpanel_apache_handler.fixture.extension
}
`, extension, handler),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_apache_handler.test",
						"extension",
						extension,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_apache_handler.test",
						"handler",
						handler,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_apache_handler.test",
						"origin",
						"user",
					),
					testAccCheckApacheHandlerExists(
						cpanelapachehandler.Definition{
							Extension: extension,
							Handler:   handler,
						},
					),
				),
			},
		},
	})
}
