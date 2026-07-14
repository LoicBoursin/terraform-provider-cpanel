package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestAccEmailAutoResponderDataSource(t *testing.T) {
	address := testAccEmailAutoResponderAddress(t, "datasource")
	autoResponder := cpanelmail.AutoResponder{
		Email:    address,
		From:     "Terraform Data Source",
		Subject:  "Data source automatic reply",
		Body:     "This message exercises the data source.",
		Charset:  "UTF-8",
		Interval: 12,
		IsHTML:   0,
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailAutoRespondersDestroyed(
			address,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailAutoResponderDataSourceConfig(autoResponder),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						"data.cpanel_email_auto_responder.test",
						"email",
						address,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_auto_responder.test",
						"subject",
						autoResponder.Subject,
					),
					resource.TestCheckResourceAttr(
						"data.cpanel_email_auto_responder.test",
						"interval_hours",
						"12",
					),
					testAccCheckEmailAutoResponderExists(autoResponder),
				),
			},
		},
	})
}

func testAccEmailAutoResponderDataSourceConfig(
	autoResponder cpanelmail.AutoResponder,
) string {
	return testAccEmailAutoResponderResourceConfig(autoResponder) + `
data "cpanel_email_auto_responder" "test" {
  email = cpanel_email_auto_responder.test.email
}
`
}
