package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

func TestSingletonUpdatesRefuseRemoteDrift(t *testing.T) {
	t.Parallel()

	t.Run("email account suspension", func(t *testing.T) {
		t.Parallel()

		client := &fakeEmailAccountSuspensionClient{
			account: emailAccountWithSuspensions(
				"mail@example.test",
				cpanelmail.AccountSuspensions{Login: true},
			),
		}
		resource := &emailAccountSuspensionResource{client: client}
		state := EmailAccountSuspensionResourceModel{
			Email:             types.StringValue("mail@example.test"),
			LoginSuspended:    types.BoolValue(false),
			IncomingSuspended: types.BoolValue(false),
			OutgoingSuspended: types.BoolValue(false),
			OutgoingHeld:      types.BoolValue(false),
			HasSuspended:      types.BoolValue(false),
		}
		plan := state
		plan.OutgoingSuspended = types.BoolValue(true)

		response := runSingletonUpdate(
			t,
			NewEmailAccountSuspensionResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.mutations) != 0 {
			t.Fatalf("mutation calls = %#v, want none", client.mutations)
		}
	})

	t.Run("locale", func(t *testing.T) {
		t.Parallel()

		client := &fakeLocaleResourceClient{
			current: cpanellocale.Locale{Code: "de"},
		}
		resource := &localeResource{client: client}
		state := LocaleResourceModel{
			Locale:        types.StringValue("fr"),
			RestoreLocale: types.StringValue("en"),
		}
		plan := state
		plan.Locale = types.StringValue("en")

		response := runSingletonUpdate(
			t,
			NewLocaleResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.setCalls) != 0 {
			t.Fatalf("Set() calls = %#v, want none", client.setCalls)
		}
	})

	t.Run("log settings", func(t *testing.T) {
		t.Parallel()

		client := &fakeLogSettingsClient{
			current: cpanellogmanager.Settings{
				ArchiveLogs:   false,
				PruneArchive:  true,
				RetentionDays: 30,
			},
		}
		resource := &logSettingsResource{client: client}
		state := LogSettingsResourceModel{
			ArchiveLogs:          types.BoolValue(true),
			PruneArchives:        types.BoolValue(true),
			RetentionDays:        types.Int64Value(30),
			RestoreArchiveLogs:   types.BoolValue(true),
			RestorePruneArchives: types.BoolValue(true),
			RestoreRetentionDays: types.Int64Value(30),
		}
		plan := state
		plan.RetentionDays = types.Int64Value(7)

		response := runSingletonUpdate(
			t,
			NewLogSettingsResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.setCalls) != 0 {
			t.Fatalf("Set() calls = %#v, want none", client.setCalls)
		}
	})

	t.Run("notification preferences", func(t *testing.T) {
		t.Parallel()

		statePreferences := map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": true,
		}
		current := testNotificationPreferences(statePreferences)
		current.Preferences["notify_disk_limit"] = false
		client := &fakeNotificationPreferencesClient{current: current}
		resource := &notificationPreferencesResource{client: client}
		state := NotificationPreferencesResourceModel{
			Preferences:  testBoolMapValue(t, statePreferences),
			Descriptions: types.MapNull(types.StringType),
			RestorePreferences: testBoolMapValue(
				t,
				statePreferences,
			),
		}
		plan := state
		plan.Preferences = testBoolMapValue(t, map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": false,
		})

		response := runSingletonUpdate(
			t,
			NewNotificationPreferencesResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.setCalls) != 0 {
			t.Fatalf(
				"SetNotificationPreferences() calls = %#v, want none",
				client.setCalls,
			)
		}
	})

	t.Run("email routing", func(t *testing.T) {
		t.Parallel()

		current := testEmailRouting(cpanelmail.RoutingModeBackup)
		client := &fakeEmailRoutingClient{current: &current}
		resource := &emailRoutingResource{client: client}
		state := EmailRoutingResourceModel{
			Domain:      types.StringValue(current.Domain),
			Mode:        types.StringValue(string(cpanelmail.RoutingModeAuto)),
			RestoreMode: types.StringValue(string(cpanelmail.RoutingModeLocal)),
		}
		plan := state
		plan.Mode = types.StringValue(string(cpanelmail.RoutingModeRemote))

		response := runSingletonUpdate(
			t,
			NewEmailRoutingResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.setCalls) != 0 {
			t.Fatalf("SetRouting() calls = %#v, want none", client.setCalls)
		}
	})

	t.Run("BoxTrapper settings", func(t *testing.T) {
		t.Parallel()

		stateSettings := testBoxTrapperSettings()
		state := BoxTrapperSettingsResourceModel{}
		applyBoxTrapperSettingsToResourceModel(&state, stateSettings)
		applyBoxTrapperRestoreToResourceModel(&state, stateSettings)
		current := stateSettings
		current.QueueDays++
		client := &fakeBoxTrapperSettingsClient{current: &current}
		resource := &boxTrapperSettingsResource{client: client}
		plan := state
		plan.Enabled = types.BoolValue(true)

		response := runSingletonUpdate(
			t,
			NewBoxTrapperSettingsResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.calls) != 0 {
			t.Fatalf("mutation calls = %#v, want none", client.calls)
		}
	})

	t.Run("SpamAssassin preference", func(t *testing.T) {
		t.Parallel()

		const preference = cpanelspam.PreferenceRequiredScore
		client := &fakeSpamPreferenceClient{
			current: cpanelspam.Preference{
				Name:    preference,
				Values:  []string{"7"},
				Present: true,
			},
		}
		resource := &spamPreferenceResource{client: client}
		state := SpamPreferenceResourceModel{
			Preference:     types.StringValue(preference),
			Values:         testStringSetValue(t, []string{"5"}),
			Configured:     types.BoolValue(true),
			RestorePresent: types.BoolValue(true),
			RestoreValues:  testStringSetValue(t, []string{"4"}),
		}
		plan := state
		plan.Values = testStringSetValue(t, []string{"6"})

		response := runSingletonUpdate(
			t,
			NewSpamPreferenceResource(),
			state,
			plan,
			resource.Update,
		)
		assertUpdateDriftRefused(t, response.Diagnostics)
		if len(client.calls) != 0 {
			t.Fatalf("mutation calls = %#v, want none", client.calls)
		}
	})
}

func testBoolMapValue(
	t *testing.T,
	values map[string]bool,
) types.Map {
	t.Helper()

	value, diagnostics := types.MapValueFrom(
		t.Context(),
		types.BoolType,
		values,
	)
	if diagnostics.HasError() {
		t.Fatalf("types.MapValueFrom() diagnostics: %v", diagnostics)
	}

	return value
}

func testStringSetValue(
	t *testing.T,
	values []string,
) types.Set {
	t.Helper()

	value, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		values,
	)
	if diagnostics.HasError() {
		t.Fatalf("types.SetValueFrom() diagnostics: %v", diagnostics)
	}

	return value
}

type fakeLocaleResourceClient struct {
	current  cpanellocale.Locale
	setCalls []string
}

func (c *fakeLocaleResourceClient) GetCurrent(
	context.Context,
) (*cpanellocale.Locale, error) {
	current := c.current

	return &current, nil
}

func (c *fakeLocaleResourceClient) Set(
	_ context.Context,
	code string,
) (*cpanellocale.Locale, error) {
	c.setCalls = append(c.setCalls, code)
	c.current.Code = code
	current := c.current

	return &current, nil
}

type fakeEmailAccountSuspensionClient struct {
	account   cpanelmail.Account
	mutations []string
}

func (c *fakeEmailAccountSuspensionClient) ListMailDomains(
	context.Context,
) ([]string, error) {
	return []string{"example.test"}, nil
}

func (c *fakeEmailAccountSuspensionClient) GetAccount(
	context.Context,
	string,
	string,
) (*cpanelmail.Account, error) {
	account := c.account

	return &account, nil
}

func (c *fakeEmailAccountSuspensionClient) SetLoginSuspended(
	context.Context,
	string,
	bool,
) error {
	c.mutations = append(c.mutations, "login")

	return nil
}

func (c *fakeEmailAccountSuspensionClient) SetIncomingSuspended(
	context.Context,
	string,
	bool,
) error {
	c.mutations = append(c.mutations, "incoming")

	return nil
}

func (c *fakeEmailAccountSuspensionClient) SetOutgoingSuspended(
	context.Context,
	string,
	bool,
) error {
	c.mutations = append(c.mutations, "outgoing")

	return nil
}

func emailAccountWithSuspensions(
	address string,
	suspensions cpanelmail.AccountSuspensions,
) cpanelmail.Account {
	return cpanelmail.Account{
		Email:                address,
		SuspendedLoginRaw:    boolFlagJSON(suspensions.Login),
		SuspendedIncomingRaw: boolFlagJSON(suspensions.Incoming),
		SuspendedOutgoingRaw: boolFlagJSON(suspensions.Outgoing),
		HoldOutgoingRaw:      boolFlagJSON(suspensions.OutgoingHeld),
		HasSuspendedRaw: boolFlagJSON(
			suspensions.Login ||
				suspensions.Incoming ||
				suspensions.Outgoing ||
				suspensions.OutgoingHeld,
		),
	}
}

func boolFlagJSON(value bool) json.RawMessage {
	if value {
		return json.RawMessage(`1`)
	}

	return json.RawMessage(`0`)
}

var (
	_ localeResourceClient          = (*fakeLocaleResourceClient)(nil)
	_ emailAccountSuspensionClient  = (*fakeEmailAccountSuspensionClient)(nil)
	_ logSettingsClient             = (*fakeLogSettingsClient)(nil)
	_ notificationPreferencesClient = (*fakeNotificationPreferencesClient)(nil)
	_ emailRoutingClient            = (*fakeEmailRoutingClient)(nil)
	_ boxTrapperSettingsClient      = (*fakeBoxTrapperSettingsClient)(nil)
	_ spamPreferenceClient          = (*fakeSpamPreferenceClient)(nil)
)
