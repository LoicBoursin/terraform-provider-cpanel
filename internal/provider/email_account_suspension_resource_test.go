package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

const testAccEmailAccountSuspensionPassword = "T9!emailSuspensionFixture-2026"

func TestEmailAccountSuspensionResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewEmailAccountSuspensionResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	email, ok := response.Schema.Attributes["email"].(resourceschema.StringAttribute)
	if !ok || !email.Required {
		t.Fatal("email must be a required string")
	}
	for _, attributeName := range []string{
		"login_suspended",
		"incoming_suspended",
		"outgoing_suspended",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.BoolAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required bool", attributeName)
		}
	}
	for _, attributeName := range []string{
		"outgoing_held",
		"has_suspended",
	} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.BoolAttribute)
		if !ok || !attribute.Computed {
			t.Fatalf("%s must be a computed bool", attributeName)
		}
	}
}

func TestManagedEmailAccountSuspensionsEqual(t *testing.T) {
	t.Parallel()

	base := cpanelmail.AccountSuspensions{
		Login:    true,
		Incoming: false,
		Outgoing: true,
	}
	if !managedEmailAccountSuspensionsEqual(
		base,
		cpanelmail.AccountSuspensions{
			Login:        true,
			Incoming:     false,
			Outgoing:     true,
			OutgoingHeld: true,
			HasSuspended: true,
		},
	) {
		t.Fatal("reported hold metadata must not affect managed suspension equality")
	}
	if managedEmailAccountSuspensionsEqual(
		base,
		cpanelmail.AccountSuspensions{
			Login:    true,
			Incoming: true,
			Outgoing: true,
		},
	) {
		t.Fatal("managed suspension drift must be detected")
	}
}

func TestAccEmailAccountSuspensionResource(t *testing.T) {
	const (
		resourceName = "cpanel_email_account_suspension.test"
	)

	firstAddress := testAccEmailAddress(t, "suspensionprimary")
	secondAddress := testAccEmailAddress(t, "suspensionreplacement")

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailAccountsDestroyed(
			firstAddress,
			secondAddress,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccEmailAccountSuspensionFixturesConfig(
					firstAddress,
					secondAddress,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailAccountSuspensions(
						firstAddress,
						cpanelmail.AccountSuspensions{},
					),
					testAccCheckEmailAccountSuspensions(
						secondAddress,
						cpanelmail.AccountSuspensions{},
					),
				),
			},
			{
				Config: testAccEmailAccountSuspensionResourceConfig(
					firstAddress,
					secondAddress,
					"primary",
					cpanelmail.AccountSuspensions{
						Login:    true,
						Outgoing: true,
					},
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"email",
						firstAddress,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"login_suspended",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"incoming_suspended",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"outgoing_suspended",
						"true",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"outgoing_held",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"has_suspended",
						"true",
					),
					testAccCheckEmailAccountSuspensions(
						firstAddress,
						cpanelmail.AccountSuspensions{
							Login:        true,
							Outgoing:     true,
							HasSuspended: true,
						},
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        firstAddress,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "email",
			},
			{
				Config: testAccEmailAccountSuspensionResourceConfig(
					firstAddress,
					secondAddress,
					"primary",
					cpanelmail.AccountSuspensions{
						Login:    true,
						Incoming: true,
						Outgoing: true,
					},
				),
				Check: testAccCheckEmailAccountSuspensions(
					firstAddress,
					cpanelmail.AccountSuspensions{
						Login:        true,
						Incoming:     true,
						Outgoing:     true,
						HasSuspended: true,
					},
				),
			},
			{
				PreConfig: func() {
					testAccSetEmailAccountSuspensions(
						t,
						firstAddress,
						cpanelmail.AccountSuspensions{
							Login:    true,
							Incoming: false,
							Outgoing: true,
						},
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailAccountSuspensionResourceConfig(
					firstAddress,
					secondAddress,
					"primary",
					cpanelmail.AccountSuspensions{
						Login:    true,
						Incoming: true,
						Outgoing: true,
					},
				),
				Check: testAccCheckEmailAccountSuspensions(
					firstAddress,
					cpanelmail.AccountSuspensions{
						Login:        true,
						Incoming:     true,
						Outgoing:     true,
						HasSuspended: true,
					},
				),
			},
			{
				Config: testAccEmailAccountSuspensionResourceConfig(
					firstAddress,
					secondAddress,
					"replacement",
					cpanelmail.AccountSuspensions{
						Incoming: true,
						Outgoing: true,
					},
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailAccountSuspensions(
						firstAddress,
						cpanelmail.AccountSuspensions{},
					),
					testAccCheckEmailAccountSuspensions(
						secondAddress,
						cpanelmail.AccountSuspensions{
							Incoming:     true,
							Outgoing:     true,
							HasSuspended: true,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailAccount(t, secondAddress)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailAccountSuspensionResourceConfig(
					firstAddress,
					secondAddress,
					"replacement",
					cpanelmail.AccountSuspensions{
						Incoming: true,
						Outgoing: true,
					},
				),
				Check: testAccCheckEmailAccountSuspensions(
					secondAddress,
					cpanelmail.AccountSuspensions{
						Incoming:     true,
						Outgoing:     true,
						HasSuspended: true,
					},
				),
			},
		},
	})
}

func testAccEmailAccountSuspensionFixturesConfig(
	firstAddress string,
	secondAddress string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "primary" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 25
  delete_on_destroy = true
}

resource "cpanel_email_account" "replacement" {
  email            = %q
  password         = %q
  password_version = 1
  quota_mib        = 25
  delete_on_destroy = true
}
`,
		firstAddress,
		testAccEmailAccountSuspensionPassword,
		secondAddress,
		testAccEmailAccountSuspensionPassword,
	)
}

func testAccEmailAccountSuspensionResourceConfig(
	firstAddress string,
	secondAddress string,
	target string,
	suspensions cpanelmail.AccountSuspensions,
) string {
	return testAccEmailAccountSuspensionFixturesConfig(
		firstAddress,
		secondAddress,
	) + fmt.Sprintf(`
resource "cpanel_email_account_suspension" "test" {
  email              = cpanel_email_account.%s.email
  login_suspended    = %t
  incoming_suspended = %t
  outgoing_suspended = %t
}
`,
		target,
		suspensions.Login,
		suspensions.Incoming,
		suspensions.Outgoing,
	)
}

func testAccCheckEmailAccountSuspensions(
	address string,
	expected cpanelmail.AccountSuspensions,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		account, err := testAccGetEmailAccount(address)
		if err != nil {
			return err
		}
		if account == nil {
			return fmt.Errorf("email account %q was not found", address)
		}
		actual, err := account.Suspensions()
		if err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf(
				"email account %q suspensions = %#v, want %#v",
				address,
				actual,
				expected,
			)
		}

		return nil
	}
}

func testAccGetEmailAccount(
	address string,
) (*cpanelmail.Account, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		return nil, err
	}
	user, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		return nil, err
	}

	return cpanelmail.NewClient(client).GetAccount(ctx, user, domain)
}

func testAccSetEmailAccountSuspensions(
	t *testing.T,
	address string,
	target cpanelmail.AccountSuspensions,
) {
	t.Helper()

	account, err := testAccGetEmailAccount(address)
	if err != nil {
		t.Fatalf("get email account %q: %v", address, err)
	}
	if account == nil {
		t.Fatalf("email account %q was not found", address)
	}
	current, err := account.Suspensions()
	if err != nil {
		t.Fatalf("read email account %q suspensions: %v", address, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	emailClient := cpanelmail.NewClient(client)
	if current.Login != target.Login {
		if err := emailClient.SetLoginSuspended(
			ctx,
			address,
			target.Login,
		); err != nil {
			t.Fatalf("set login suspension for %q: %v", address, err)
		}
	}
	if current.Incoming != target.Incoming {
		if err := emailClient.SetIncomingSuspended(
			ctx,
			address,
			target.Incoming,
		); err != nil {
			t.Fatalf("set incoming suspension for %q: %v", address, err)
		}
	}
	if current.Outgoing != target.Outgoing {
		if err := emailClient.SetOutgoingSuspended(
			ctx,
			address,
			target.Outgoing,
		); err != nil {
			t.Fatalf("set outgoing suspension for %q: %v", address, err)
		}
	}
}
