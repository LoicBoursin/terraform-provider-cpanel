package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccEmailMailingListDataSource(t *testing.T) {
	const (
		resourceName   = "cpanel_email_mailing_list.fixture"
		dataSourceName = "data.cpanel_email_mailing_list.test"
	)

	address := testAccEmailMailingListAddress(t, "datasource")
	password := "Tf9!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailMailingListsDestroyed(address),
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "fixture" {
  address          = %q
  password         = %q
  password_version = 1
  delete_on_destroy = true
  advertised       = true
  archive_private  = true
  subscribe_policy = 3
}

data "cpanel_email_mailing_list" "test" {
  address = cpanel_email_mailing_list.fixture.address
}
`, address, password),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						dataSourceName,
						"address",
						address,
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"private",
						"false",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"advertised",
						"true",
					),
					resource.TestCheckResourceAttr(
						dataSourceName,
						"archive_private",
						"true",
					),
					resource.TestCheckResourceAttrSet(
						dataSourceName,
						"list_id",
					),
					resource.TestCheckResourceAttrPair(
						dataSourceName,
						"list_id",
						resourceName,
						"list_id",
					),
				),
			},
		},
	})
}
