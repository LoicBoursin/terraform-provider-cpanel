package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestAccEmailMailingListResource(t *testing.T) {
	const resourceName = "cpanel_email_mailing_list.test"

	address := testAccEmailMailingListAddress(t, "resource")
	initialPassword := "Tf9!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)
	updatedPassword := "Rg8!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)
	delegateAddress := testAccEmailAddress(t, "mailmandelegate")
	delegatePassword := "Dl7!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)
	private := cpanelmail.MailingListPrivacyOptions{
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	mixedPublic := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	drifted := cpanelmail.MailingListPrivacyOptions{
		ArchivePrivate:  false,
		SubscribePolicy: 2,
	}
	var initialListID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckEmailMailingListsDestroyed(address),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailMailingListResourceConfig(
					address,
					initialPassword,
					1,
					private,
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("password"),
						knownvalue.Null(),
					),
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("password_version"),
						knownvalue.Int64Exact(1),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"address",
						address,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"private",
						"true",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"advertised",
						"false",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"archive_private",
						"true",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"subscribe_policy",
						"3",
					),
					resource.TestCheckResourceAttrSet(
						resourceName,
						"list_id",
					),
					testAccCheckEmailMailingList(
						address,
						private,
						nil,
						&initialListID,
					),
					testAccCheckEmailMailingListPassword(
						address,
						initialPassword,
						true,
					),
				),
			},
			{
				PreConfig: func() {
					testAccEnsureEmailMailingListDelegate(
						t,
						address,
						delegateAddress,
						delegatePassword,
					)
				},
				Config: testAccEmailMailingListResourceConfig(
					address,
					updatedPassword,
					2,
					private,
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("password"),
						knownvalue.Null(),
					),
					statecheck.ExpectKnownValue(
						resourceName,
						tfjsonpath.New("password_version"),
						knownvalue.Int64Exact(2),
					),
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"password_version",
						"2",
					),
					testAccCheckEmailMailingList(
						address,
						private,
						&initialListID,
						nil,
					),
					testAccCheckEmailMailingListPassword(
						address,
						updatedPassword,
						true,
					),
					testAccCheckEmailMailingListPassword(
						address,
						initialPassword,
						false,
					),
					testAccCheckEmailMailingListDelegate(
						address,
						delegateAddress,
					),
				),
			},
			{
				Config: testAccEmailMailingListResourceConfig(
					address,
					updatedPassword,
					2,
					mixedPublic,
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"private",
						"false",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"advertised",
						"true",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"archive_private",
						"true",
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"subscribe_policy",
						"3",
					),
					testAccCheckEmailMailingList(
						address,
						mixedPublic,
						&initialListID,
						nil,
					),
					testAccCheckEmailMailingListDelegate(
						address,
						delegateAddress,
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        address,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "address",
				ImportStateVerifyIgnore: []string{
					"password",
					"password_version",
					"delete_on_destroy",
				},
			},
			{
				PreConfig: func() {
					testAccSetEmailMailingListPrivacy(
						t,
						address,
						drifted,
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailMailingListResourceConfig(
					address,
					updatedPassword,
					2,
					mixedPublic,
				),
				Check: testAccCheckEmailMailingList(
					address,
					mixedPublic,
					&initialListID,
					nil,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailMailingList(t, address)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailMailingListResourceConfig(
					address,
					updatedPassword,
					2,
					mixedPublic,
				),
				Check: testAccCheckEmailMailingList(
					address,
					mixedPublic,
					nil,
					nil,
				),
			},
		},
	})
}

func TestAccEmailMailingListResourceImport(t *testing.T) {
	const resourceName = "cpanel_email_mailing_list.test"

	address := testAccEmailMailingListAddress(t, "import")
	password := "Tf9!" + acctest.RandStringFromCharSet(
		24,
		acctest.CharSetAlphaNum,
	)
	mixedPublic := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	testAccCreateEmailMailingList(t, address, password, mixedPublic)
	t.Cleanup(func() {
		testAccDeleteEmailMailingList(t, address)
	})

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEmailMailingListImportedConfig(
					address,
					mixedPublic,
				),
				ResourceName:       resourceName,
				ImportStateId:      address,
				ImportState:        true,
				ImportStatePersist: true,
			},
			{
				Config: testAccEmailMailingListImportedConfig(
					address,
					mixedPublic,
				),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: testAccCheckEmailMailingList(
					address,
					mixedPublic,
					nil,
					nil,
				),
			},
		},
	})
}

func TestAccEmailMailingListPasswordConfigurationValidation(t *testing.T) {
	address := testAccEmailMailingListAddress(t, "validation")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "test" {
  address          = %q
  password         = "Tf9!password-only"
  advertised       = false
  archive_private  = true
  subscribe_policy = 3
}
`, address),
				ExpectError: regexp.MustCompile(`password_version`),
			},
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "test" {
  address          = %q
  password_version = 1
  advertised       = false
  archive_private  = true
  subscribe_policy = 3
}
`, address),
				ExpectError: regexp.MustCompile(`password`),
			},
			{
				Config: providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "test" {
  address          = %q
  password         = "Tf9!invalid-version"
  password_version = 0
  advertised       = false
  archive_private  = true
  subscribe_policy = 3
}
`, address),
				ExpectError: regexp.MustCompile(`at least 1`),
			},
		},
	})
}

func testAccEmailMailingListResourceConfig(
	address string,
	password string,
	passwordVersion int64,
	privacy cpanelmail.MailingListPrivacyOptions,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "test" {
  address          = %q
  password         = %q
  password_version = %d
  delete_on_destroy = true
  advertised       = %t
  archive_private  = %t
  subscribe_policy = %d
}
`,
		address,
		password,
		passwordVersion,
		privacy.Advertised,
		privacy.ArchivePrivate,
		privacy.SubscribePolicy,
	)
}

func testAccEmailMailingListImportedConfig(
	address string,
	privacy cpanelmail.MailingListPrivacyOptions,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_mailing_list" "test" {
  address          = %q
  advertised       = %t
  archive_private  = %t
  subscribe_policy = %d
}
`,
		address,
		privacy.Advertised,
		privacy.ArchivePrivate,
		privacy.SubscribePolicy,
	)
}
