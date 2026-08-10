package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

func TestAccCalendarDelegateResource(t *testing.T) {
	const resourceName = "cpanel_calendar_delegate.test"

	owner := testAccEmailAddress(t, "calowner")
	delegate := testAccEmailAddress(t, "caldelegate")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCalendarDelegatePreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeAggregateTestCheckFunc(
			testAccCheckCalendarDelegatesDestroyed(owner, delegate),
			testAccCheckEmailAccountsDestroyed(owner, delegate),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"delegator",
						owner,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"delegatee",
						delegate,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"calendar",
						cpanelcalendar.DefaultCalendar,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"readonly",
						"true",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"calendar_name",
					),
					testAccCheckCalendarDelegate(
						owner,
						delegate,
						true,
					),
					testresource.TestCheckResourceAttr(
						"data.cpanel_calendar_delegate.test",
						"readonly",
						"true",
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        calendarDelegateImportID(owner, delegate),
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "delegator",
			},
			{
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					false,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"readonly",
						"false",
					),
					testAccCheckCalendarDelegate(
						owner,
						delegate,
						false,
					),
				),
			},
			{
				PreConfig: func() {
					testAccUpdateCalendarDelegate(t, owner, delegate, true)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					false,
				),
				Check: testAccCheckCalendarDelegate(
					owner,
					delegate,
					false,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteCalendarDelegate(t, owner, delegate)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					false,
				),
				Check: testAccCheckCalendarDelegate(
					owner,
					delegate,
					false,
				),
			},
		},
	})
}

func TestAccCalendarDelegateResourceRefusesImplicitTakeover(t *testing.T) {
	owner := testAccEmailAddress(t, "calownercollision")
	delegate := testAccEmailAddress(t, "caldelegatecollision")

	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccCalendarDelegatePreCheck(t)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testresource.ComposeAggregateTestCheckFunc(
			testAccCheckCalendarDelegatesDestroyed(owner, delegate),
			testAccCheckEmailAccountsDestroyed(owner, delegate),
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccCalendarDelegateFixtureConfig(owner, delegate),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckEmailAccountExists(owner, 25),
					testAccCheckEmailAccountExists(delegate, 25),
				),
			},
			{
				PreConfig: func() {
					testAccCreateCalendarDelegate(t, owner, delegate, true)
				},
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					true,
				),
				ExpectError: regexp.MustCompile(
					`Calendar delegate already exists`,
				),
			},
			{
				PreConfig: func() {
					testAccDeleteCalendarDelegate(t, owner, delegate)
				},
				Config: testAccCalendarDelegateResourceConfig(
					owner,
					delegate,
					true,
				),
				Check: testAccCheckCalendarDelegate(
					owner,
					delegate,
					true,
				),
			},
		},
	})
}

func testAccCalendarDelegateFixtureConfig(
	owner string,
	delegate string,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_account" "calendar_owner" {
  email            = %q
  password         = "H7!calendarOwner-2026"
  password_version = 1
  quota_mib        = 25
  delete_on_destroy = true
}

resource "cpanel_email_account" "calendar_delegate" {
  email            = %q
  password         = "N9!calendarDelegate-2026"
  password_version = 1
  quota_mib        = 25
  delete_on_destroy = true
}
`, owner, delegate)
}

func testAccCalendarDelegateResourceConfig(
	owner string,
	delegate string,
	readonly bool,
) string {
	return testAccCalendarDelegateFixtureConfig(owner, delegate) + fmt.Sprintf(`
resource "cpanel_calendar_delegate" "test" {
  delegator = cpanel_email_account.calendar_owner.email
  delegatee = cpanel_email_account.calendar_delegate.email
  readonly  = %t
}

data "cpanel_calendar_delegate" "test" {
  delegator = cpanel_calendar_delegate.test.delegator
  delegatee = cpanel_calendar_delegate.test.delegatee

  depends_on = [cpanel_calendar_delegate.test]
}
`, readonly)
}

func testAccCalendarDelegatePreCheck(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if _, err := cpanelcalendar.NewClient(client).ListDelegates(ctx); err != nil {
		t.Fatalf("verify calendar delegation API access: %v", err)
	}
}

func testAccCheckCalendarDelegate(
	owner string,
	delegate string,
	readonly bool,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		actual, err := cpanelcalendar.NewClient(client).GetDelegate(
			ctx,
			owner,
			cpanelcalendar.DefaultCalendar,
			delegate,
		)
		if err != nil {
			return err
		}
		if actual == nil {
			return fmt.Errorf(
				"calendar delegate from %q to %q was not found",
				owner,
				delegate,
			)
		}
		if actual.ReadOnly != readonly {
			return fmt.Errorf(
				"calendar delegate readonly = %t, want %t",
				actual.ReadOnly,
				readonly,
			)
		}

		return nil
	}
}

func testAccCheckCalendarDelegatesDestroyed(
	owner string,
	delegate string,
) testresource.TestCheckFunc {
	return func(_ *terraform.State) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		client, err := testAccClient()
		if err != nil {
			return err
		}
		actual, err := cpanelcalendar.NewClient(client).GetDelegate(
			ctx,
			owner,
			cpanelcalendar.DefaultCalendar,
			delegate,
		)
		if err != nil {
			return err
		}
		if actual != nil {
			return fmt.Errorf(
				"calendar delegate from %q to %q still exists",
				owner,
				delegate,
			)
		}

		return nil
	}
}

func testAccCreateCalendarDelegate(
	t *testing.T,
	owner string,
	delegate string,
	readonly bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelcalendar.NewClient(client).AddDelegate(
		ctx,
		cpanelcalendar.Definition{
			Delegator: owner,
			Delegatee: delegate,
			Calendar:  cpanelcalendar.DefaultCalendar,
			ReadOnly:  readonly,
		},
	); err != nil {
		t.Fatalf(
			"create calendar delegate from %q to %q: %v",
			owner,
			delegate,
			err,
		)
	}
}

func testAccUpdateCalendarDelegate(
	t *testing.T,
	owner string,
	delegate string,
	readonly bool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	if err := cpanelcalendar.NewClient(client).UpdateDelegate(
		ctx,
		cpanelcalendar.Definition{
			Delegator: owner,
			Delegatee: delegate,
			Calendar:  cpanelcalendar.DefaultCalendar,
			ReadOnly:  readonly,
		},
	); err != nil {
		t.Fatalf(
			"update calendar delegate from %q to %q: %v",
			owner,
			delegate,
			err,
		)
	}
}

func testAccDeleteCalendarDelegate(
	t *testing.T,
	owner string,
	delegate string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	calendarClient := cpanelcalendar.NewClient(client)
	actual, err := calendarClient.GetDelegate(
		ctx,
		owner,
		cpanelcalendar.DefaultCalendar,
		delegate,
	)
	if err != nil {
		t.Fatalf(
			"read calendar delegate from %q to %q: %v",
			owner,
			delegate,
			err,
		)
	}
	if actual == nil {
		return
	}
	if err := calendarClient.RemoveDelegate(
		ctx,
		owner,
		cpanelcalendar.DefaultCalendar,
		delegate,
	); err != nil {
		t.Fatalf(
			"delete calendar delegate from %q to %q: %v",
			owner,
			delegate,
			err,
		)
	}
}

func calendarDelegateImportID(owner string, delegate string) string {
	return owner +
		calendarDelegateImportSeparator +
		cpanelcalendar.DefaultCalendar +
		calendarDelegateImportSeparator +
		delegate
}
