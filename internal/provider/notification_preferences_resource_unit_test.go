package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelcontact "terraform-provider-cpanel/internal/cpanel/contactinformation"
)

func TestNotificationPreferencesResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewNotificationPreferencesResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	preferences, ok := response.Schema.Attributes["preferences"].(resourceschema.MapAttribute)
	if !ok || !preferences.Required ||
		preferences.ElementType != types.BoolType {
		t.Fatal("preferences must be a required bool map")
	}
	descriptions, ok := response.Schema.Attributes["descriptions"].(resourceschema.MapAttribute)
	if !ok || !descriptions.Computed ||
		descriptions.ElementType != types.StringType {
		t.Fatal("descriptions must be a computed string map")
	}
	restore, ok := response.Schema.Attributes["restore_preferences"].(resourceschema.MapAttribute)
	if !ok || !restore.Computed || restore.ElementType != types.BoolType {
		t.Fatal("restore_preferences must be a computed bool map")
	}
	if !response.Schema.Attributes["account"].IsComputed() {
		t.Fatal("account must be computed")
	}
}

func TestNotificationPreferencesTransitionRestoresAfterMutationFailure(
	t *testing.T,
) {
	t.Parallel()

	original := testNotificationPreferences(
		map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": true,
		},
	)
	target := map[string]bool{
		"notify_disk_limit": false,
		"notify_ssl_expiry": true,
	}
	client := &fakeNotificationPreferencesClient{
		current:    original,
		setErr:     errors.New("connection closed after request"),
		errMutates: true,
	}
	resource := &notificationPreferencesResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 2 {
		t.Fatalf(
			"SetNotificationPreferences() call count = %d, want 2",
			len(client.setCalls),
		)
	}
	if !cpanelcontact.PreferencesMatchDefinition(
		client.current,
		original.Definition(),
	) {
		t.Fatalf(
			"transition() left preferences %#v, want %#v",
			client.current.Preferences,
			original.Preferences,
		)
	}
}

func TestNotificationPreferencesTransitionSkipsRollbackAfterRejectedMutation(
	t *testing.T,
) {
	t.Parallel()

	original := testNotificationPreferences(
		map[string]bool{"notify_disk_limit": true},
	)
	target := map[string]bool{"notify_disk_limit": false}
	client := &fakeNotificationPreferencesClient{
		current: original,
		setErr: &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "ContactInformation",
			Function: "set_notification_preferences",
			Messages: []string{"rejected"},
		},
	}
	resource := &notificationPreferencesResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf(
			"SetNotificationPreferences() call count = %d, want 1",
			len(client.setCalls),
		)
	}
	if !reflect.DeepEqual(client.current, original) {
		t.Fatalf(
			"transition() changed preferences to %#v, want %#v",
			client.current,
			original,
		)
	}
}

func TestNotificationPreferencesTransitionPreservesConcurrentChange(
	t *testing.T,
) {
	t.Parallel()

	original := testNotificationPreferences(
		map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": true,
		},
	)
	target := map[string]bool{
		"notify_disk_limit": false,
		"notify_ssl_expiry": true,
	}
	concurrent := testNotificationPreferences(
		map[string]bool{
			"notify_disk_limit": true,
			"notify_ssl_expiry": false,
		},
	)
	client := &fakeNotificationPreferencesClient{
		current:         original,
		setErr:          errors.New("connection closed after request"),
		stateAfterError: &concurrent,
	}
	resource := &notificationPreferencesResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf(
			"SetNotificationPreferences() call count = %d, want 1",
			len(client.setCalls),
		)
	}
	if !reflect.DeepEqual(client.current, concurrent) {
		t.Fatalf(
			"transition() changed concurrent preferences to %#v, want %#v",
			client.current,
			concurrent,
		)
	}
}

func TestNotificationPreferencesRestoreSkipsMatchingState(t *testing.T) {
	t.Parallel()

	current := testNotificationPreferences(
		map[string]bool{"notify_disk_limit": true},
	)
	client := &fakeNotificationPreferencesClient{current: current}
	resource := &notificationPreferencesResource{client: client}

	if err := resource.restore(
		t.Context(),
		current.Definition(),
	); err != nil {
		t.Fatalf("restore() error: %v", err)
	}
	if len(client.setCalls) != 0 {
		t.Fatalf(
			"SetNotificationPreferences() call count = %d, want 0",
			len(client.setCalls),
		)
	}
}

func TestNotificationPreferencesGuardedRestorePreservesConcurrentChange(
	t *testing.T,
) {
	t.Parallel()

	original := map[string]bool{
		"notify_disk_limit": true,
		"notify_ssl_expiry": true,
	}
	managed := map[string]bool{
		"notify_disk_limit": false,
		"notify_ssl_expiry": true,
	}
	concurrent := testNotificationPreferences(
		map[string]bool{
			"notify_disk_limit": false,
			"notify_ssl_expiry": false,
		},
	)
	client := &fakeNotificationPreferencesClient{current: concurrent}
	resource := &notificationPreferencesResource{client: client}

	if err := resource.restoreIfCurrentMatches(
		t.Context(),
		managed,
		original,
	); err == nil {
		t.Fatal("restoreIfCurrentMatches() returned no error")
	}
	if len(client.setCalls) != 0 {
		t.Fatalf(
			"SetNotificationPreferences() call count = %d, want 0",
			len(client.setCalls),
		)
	}
	if !reflect.DeepEqual(client.current, concurrent) {
		t.Fatalf(
			"guarded restore changed preferences to %#v, want %#v",
			client.current,
			concurrent,
		)
	}
}

type fakeNotificationPreferencesClient struct {
	current cpanelcontact.NotificationPreferences

	setErr     error
	errMutates bool
	setCalls   []map[string]bool

	stateAfterError *cpanelcontact.NotificationPreferences
}

func (c *fakeNotificationPreferencesClient) GetNotificationPreferences(
	context.Context,
) (*cpanelcontact.NotificationPreferences, error) {
	result := testNotificationPreferences(c.current.Preferences)
	result.Descriptions = cloneStringMap(c.current.Descriptions)

	return &result, nil
}

func (c *fakeNotificationPreferencesClient) SetNotificationPreferences(
	_ context.Context,
	definition map[string]bool,
) (*cpanelcontact.NotificationPreferences, error) {
	c.setCalls = append(c.setCalls, cloneBoolMap(definition))
	if c.setErr != nil {
		err := c.setErr
		c.setErr = nil
		if c.errMutates {
			c.current.Preferences = cloneBoolMap(definition)
		} else if c.stateAfterError != nil {
			c.current = *c.stateAfterError
		}

		return nil, err
	}

	c.current.Preferences = cloneBoolMap(definition)
	result := testNotificationPreferences(c.current.Preferences)
	result.Descriptions = cloneStringMap(c.current.Descriptions)

	return &result, nil
}

func testNotificationPreferences(
	preferences map[string]bool,
) cpanelcontact.NotificationPreferences {
	descriptions := make(map[string]string, len(preferences))
	for name := range preferences {
		descriptions[name] = name + " description"
	}

	return cpanelcontact.NotificationPreferences{
		Preferences:  cloneBoolMap(preferences),
		Descriptions: descriptions,
	}
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for name, value := range source {
		result[name] = value
	}

	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for name, value := range source {
		result[name] = value
	}

	return result
}
