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
	cpanelspam "terraform-provider-cpanel/internal/cpanel/spamassassin"
)

func TestSpamPreferenceResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewSpamPreferenceResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	values, ok := response.Schema.Attributes["values"].(resourceschema.SetAttribute)
	if !ok || !values.Required || values.ElementType != types.StringType {
		t.Fatal("values must be a required string set")
	}
	restoreValues, ok := response.Schema.Attributes["restore_values"].(resourceschema.SetAttribute)
	if !ok || !restoreValues.Computed ||
		restoreValues.ElementType != types.StringType {
		t.Fatal("restore_values must be a computed string set")
	}
	for _, attribute := range []string{
		"configured",
		"restore_present",
	} {
		if !response.Schema.Attributes[attribute].IsComputed() {
			t.Fatalf("%s must be computed", attribute)
		}
	}
}

func TestSpamPreferenceTransitionRestoresAfterAmbiguousMutation(
	t *testing.T,
) {
	t.Parallel()

	original := cpanelspam.Preference{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{},
		Present: false,
	}
	target := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"5.5"},
		Present: true,
	}
	client := &fakeSpamPreferenceClient{
		current:    original,
		setErr:     errors.New("connection closed after request"),
		errMutates: true,
	}
	resource := &spamPreferenceResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.calls) != 2 ||
		!client.calls[0].Present ||
		client.calls[1].Present {
		t.Fatalf("mutation calls = %#v, want set then remove", client.calls)
	}
	if client.current.Present {
		t.Fatalf("transition() left preference %#v, want absent", client.current)
	}
}

func TestSpamPreferenceTransitionSkipsRollbackAfterRejectedMutation(
	t *testing.T,
) {
	t.Parallel()

	original := cpanelspam.Preference{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"5"},
		Present: true,
	}
	target := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"6"},
		Present: true,
	}
	client := &fakeSpamPreferenceClient{
		current: original,
		setErr: &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "SpamAssassin",
			Function: "update_user_preference",
			Messages: []string{"rejected"},
		},
	}
	resource := &spamPreferenceResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.calls) != 1 {
		t.Fatalf("mutation calls = %#v, want one set", client.calls)
	}
	if !reflect.DeepEqual(client.current, original) {
		t.Fatalf(
			"transition() changed preference to %#v, want %#v",
			client.current,
			original,
		)
	}
}

func TestSpamPreferenceGuardedRestorePreservesConcurrentChange(
	t *testing.T,
) {
	t.Parallel()

	managed := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"6"},
		Present: true,
	}
	restore := cpanelspam.Definition{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"5"},
		Present: true,
	}
	concurrent := cpanelspam.Preference{
		Name:    cpanelspam.PreferenceRequiredScore,
		Values:  []string{"7"},
		Present: true,
	}
	client := &fakeSpamPreferenceClient{current: concurrent}
	resource := &spamPreferenceResource{client: client}

	if err := resource.restoreIfCurrentMatches(
		t.Context(),
		managed,
		restore,
	); err == nil {
		t.Fatal("restoreIfCurrentMatches() returned no error")
	}
	if len(client.calls) != 0 {
		t.Fatalf("mutation calls = %#v, want none", client.calls)
	}
	if !reflect.DeepEqual(client.current, concurrent) {
		t.Fatalf(
			"guarded restore changed preference to %#v, want %#v",
			client.current,
			concurrent,
		)
	}
}

type fakeSpamPreferenceClient struct {
	current cpanelspam.Preference

	setErr          error
	removeErr       error
	errMutates      bool
	stateAfterError *cpanelspam.Preference
	calls           []cpanelspam.Definition
}

func (c *fakeSpamPreferenceClient) LockPreference(string) func() {
	return func() {}
}

func (c *fakeSpamPreferenceClient) GetPreference(
	context.Context,
	string,
) (*cpanelspam.Preference, error) {
	result := c.current
	result.Values = append([]string{}, c.current.Values...)

	return &result, nil
}

func (c *fakeSpamPreferenceClient) SetPreference(
	_ context.Context,
	name string,
	values []string,
) (*cpanelspam.Preference, error) {
	target := cpanelspam.Definition{
		Name:    name,
		Values:  append([]string{}, values...),
		Present: true,
	}.Sorted()
	c.calls = append(c.calls, target)
	if c.setErr != nil {
		err := c.setErr
		c.setErr = nil
		if c.errMutates {
			c.current = preferenceFromDefinition(target)
		} else if c.stateAfterError != nil {
			c.current = *c.stateAfterError
		}

		return nil, err
	}

	c.current = preferenceFromDefinition(target)
	result := c.current

	return &result, nil
}

func (c *fakeSpamPreferenceClient) RemovePreference(
	_ context.Context,
	name string,
) (*cpanelspam.Preference, error) {
	target := cpanelspam.Definition{
		Name:    name,
		Present: false,
	}
	c.calls = append(c.calls, target)
	if c.removeErr != nil {
		err := c.removeErr
		c.removeErr = nil

		return nil, err
	}

	c.current = preferenceFromDefinition(target)
	result := c.current

	return &result, nil
}

func preferenceFromDefinition(
	definition cpanelspam.Definition,
) cpanelspam.Preference {
	return cpanelspam.Preference{
		Name:    definition.Name,
		Values:  append([]string{}, definition.Values...),
		Present: definition.Present,
	}
}
