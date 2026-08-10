package provider

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

func TestDAVUsersDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewDAVUsersDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	users, ok := response.Schema.Attributes["users"].(datasourceschema.ListNestedAttribute)
	if !ok || !users.Computed {
		t.Fatal("users must be a computed nested list")
	}
	if len(users.NestedObject.Attributes) != 2 {
		t.Fatalf(
			"user nested attribute count = %d",
			len(users.NestedObject.Attributes),
		)
	}
}

func TestCalendarDelegatesDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &datasource.SchemaResponse{}
	NewCalendarDelegatesDataSource().Schema(
		t.Context(),
		datasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}
	delegates, ok := response.Schema.Attributes["delegates"].(datasourceschema.ListNestedAttribute)
	if !ok || !delegates.Computed {
		t.Fatal("delegates must be a computed nested list")
	}
	if len(delegates.NestedObject.Attributes) != 5 {
		t.Fatalf(
			"delegate nested attribute count = %d",
			len(delegates.NestedObject.Attributes),
		)
	}
}

func TestAccDAVInventoryDataSources(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC must be set for acceptance tests")
	}

	baseline := testAccReadDAVInventory(t)
	t.Cleanup(func() {
		testAccRequireDAVInventory(t, baseline)
	})

	const (
		usersName     = "data.cpanel_dav_users.all"
		delegatesName = "data.cpanel_calendar_delegates.all"
	)
	checks := []testresource.TestCheckFunc{
		testresource.TestCheckResourceAttr(
			usersName,
			"users.#",
			strconv.Itoa(len(baseline.Users)),
		),
		testresource.TestCheckResourceAttr(
			delegatesName,
			"delegates.#",
			strconv.Itoa(len(baseline.Delegates)),
		),
	}
	for userIndex, user := range baseline.Users {
		userPrefix := fmt.Sprintf("users.%d.", userIndex)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				usersName,
				userPrefix+"username",
				user.Username,
			),
			testresource.TestCheckResourceAttr(
				usersName,
				userPrefix+"collections.#",
				strconv.Itoa(len(user.Collections)),
			),
		)
		for collectionIndex, collection := range user.Collections {
			collectionPrefix := fmt.Sprintf(
				"%scollections.%d.",
				userPrefix,
				collectionIndex,
			)
			checks = append(
				checks,
				testresource.TestCheckResourceAttr(
					usersName,
					collectionPrefix+"name",
					collection.Name,
				),
				testresource.TestCheckResourceAttr(
					usersName,
					collectionPrefix+"display_name",
					collection.DisplayName,
				),
				testresource.TestCheckResourceAttr(
					usersName,
					collectionPrefix+"type",
					collection.Type,
				),
			)
		}
	}
	for index, delegate := range baseline.Delegates {
		prefix := fmt.Sprintf("delegates.%d.", index)
		checks = append(
			checks,
			testresource.TestCheckResourceAttr(
				delegatesName,
				prefix+"delegator",
				delegate.Delegator,
			),
			testresource.TestCheckResourceAttr(
				delegatesName,
				prefix+"delegatee",
				delegate.Delegatee,
			),
			testresource.TestCheckResourceAttr(
				delegatesName,
				prefix+"calendar",
				delegate.Calendar,
			),
			testresource.TestCheckResourceAttr(
				delegatesName,
				prefix+"calendar_name",
				delegate.CalendarName,
			),
			testresource.TestCheckResourceAttr(
				delegatesName,
				prefix+"readonly",
				strconv.FormatBool(delegate.ReadOnly),
			),
		)
	}

	const config = providerConfig + `
data "cpanel_dav_users" "all" {}

data "cpanel_calendar_delegates" "all" {}
`
	testresource.Test(t, testresource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccRequireDAVInventory(t, baseline)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []testresource.TestStep{
			{
				Config: config,
				Check: testresource.ComposeAggregateTestCheckFunc(
					checks...,
				),
			},
			{
				Config: config,
				ConfigPlanChecks: testresource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

type testAccDAVInventoryBaseline struct {
	Users     []cpanelcalendar.User
	Delegates []cpanelcalendar.Delegate
}

func testAccReadDAVInventory(
	t *testing.T,
) testAccDAVInventoryBaseline {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		90*time.Second,
	)
	defer cancel()
	client, err := testAccClient()
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}
	calendarClient := cpanelcalendar.NewClient(client)
	users, err := calendarClient.ListUsers(ctx)
	if err != nil {
		t.Fatalf("read cPanel DAV user inventory: %v", err)
	}
	delegates, err := calendarClient.ListDelegates(ctx)
	if err != nil {
		t.Fatalf("read cPanel calendar delegation inventory: %v", err)
	}

	return testAccDAVInventoryBaseline{
		Users:     users,
		Delegates: delegates,
	}
}

func testAccRequireDAVInventory(
	t *testing.T,
	expected testAccDAVInventoryBaseline,
) {
	t.Helper()

	actual := testAccReadDAVInventory(t)
	if !reflect.DeepEqual(actual.Users, expected.Users) {
		t.Fatalf(
			"cPanel DAV user inventory changed: got %d users, expected %d",
			len(actual.Users),
			len(expected.Users),
		)
	}
	if !reflect.DeepEqual(actual.Delegates, expected.Delegates) {
		t.Fatalf(
			"cPanel calendar delegation inventory changed: "+
				"got %d delegates, expected %d",
			len(actual.Delegates),
			len(expected.Delegates),
		)
	}
}
