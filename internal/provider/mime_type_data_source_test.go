package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelmimetype "terraform-provider-cpanel/internal/cpanel/mimetype"
)

func TestAccMIMETypeDataSource(t *testing.T) {
	mimeTypeName := testAccMIMEType("datasource")
	firstExtension := testAccMIMEExtension("datafirst")
	secondExtension := testAccMIMEExtension("datasecond")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMIMETypesDestroyed(mimeTypeName),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_mime_type" "fixture" {
  type       = %q
  extensions = [%q, %q]
}

data "cpanel_mime_type" "test" {
  type = cpanel_mime_type.fixture.type
}
`, mimeTypeName, firstExtension, secondExtension),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_mime_type.test",
						"type",
						mimeTypeName,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_mime_type.test",
						"extensions.#",
						"2",
					),
					resource.TestCheckTypeSetElemAttr(
						"data.cpanel_mime_type.test",
						"extensions.*",
						firstExtension,
					),
					resource.TestCheckTypeSetElemAttr(
						"data.cpanel_mime_type.test",
						"extensions.*",
						secondExtension,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_mime_type.test",
						"origin",
						"user",
					),
					testAccCheckMIMETypeExists(cpanelmimetype.Definition{
						Type: mimeTypeName,
						Extensions: []string{
							firstExtension,
							secondExtension,
						},
					}),
				),
			},
		},
	})
}
