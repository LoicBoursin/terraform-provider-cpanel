package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

func TestCalendarDelegateResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewCalendarDelegateResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, name := range []string{"delegator", "delegatee"} {
		attribute, ok := response.Schema.Attributes[name].(resourceschema.StringAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required string", name)
		}
	}

	calendar, ok := response.Schema.Attributes["calendar"].(resourceschema.StringAttribute)
	if !ok || !calendar.Optional || !calendar.Computed || calendar.Default == nil {
		t.Fatal("calendar must be an optional computed string with a default")
	}
	readonly, ok := response.Schema.Attributes["readonly"].(resourceschema.BoolAttribute)
	if !ok || !readonly.Optional || !readonly.Computed || readonly.Default == nil {
		t.Fatal("readonly must be an optional computed bool with a default")
	}
	calendarName, ok := response.Schema.Attributes["calendar_name"].(resourceschema.StringAttribute)
	if !ok || !calendarName.Computed {
		t.Fatal("calendar_name must be a computed string")
	}
}

func TestCalendarDelegateCreateDoesNotAdoptAmbiguousMutation(t *testing.T) {
	t.Parallel()

	desired := testCalendarDelegateDefinition()
	client := &fakeCalendarDelegateClient{
		addErr:     errors.New("connection closed after request"),
		addMutates: true,
	}
	resource := &calendarDelegateResource{client: client}

	actual, rollbackAllowed, err := resource.createAndVerify(
		t.Context(),
		desired,
	)
	if err == nil {
		t.Fatal("createAndVerify() returned no error")
	}
	if rollbackAllowed {
		t.Fatal("createAndVerify() allowed rollback after ambiguous mutation")
	}
	if actual != nil {
		t.Fatalf("createAndVerify() = %#v, want nil", actual)
	}
	if client.delegate == nil {
		t.Fatal("createAndVerify() removed the observed delegate")
	}
	if client.removeCalls != 0 {
		t.Fatalf("remove calls = %d, want 0", client.removeCalls)
	}
}

func TestCalendarDelegateCreateRefusesDeterministicErrorAdoption(
	t *testing.T,
) {
	t.Parallel()

	tests := map[string]error{
		"wrapped UAPI error": fmt.Errorf("wrapped: %w", &cpanel.APIError{
			API:      "UAPI",
			Module:   cpanel.ModuleCPDAVD,
			Function: "add_delegate",
			Messages: []string{"delegation rejected"},
		}),
		"wrapped HTTP error": fmt.Errorf(
			"wrapped: %w",
			&cpanel.HTTPError{StatusCode: 500},
		),
	}

	for name, mutationErr := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			desired := testCalendarDelegateDefinition()
			client := &fakeCalendarDelegateClient{
				addErr:     mutationErr,
				addMutates: true,
			}
			resource := &calendarDelegateResource{client: client}

			actual, rollbackAllowed, err := resource.createAndVerify(
				t.Context(),
				desired,
			)
			if err == nil {
				t.Fatal("createAndVerify() returned no error")
			}
			if actual != nil {
				t.Fatalf("createAndVerify() = %#v, want nil", actual)
			}
			if rollbackAllowed {
				t.Fatal("createAndVerify() allowed rollback after deterministic error")
			}
			if client.delegate == nil {
				t.Fatal("test setup did not expose the concurrent delegate")
			}
		})
	}
}

func TestCalendarDelegateCreateSkipsDisallowedRollback(t *testing.T) {
	t.Parallel()

	desired := testCalendarDelegateDefinition()
	client := &fakeCalendarDelegateClient{
		delegate: testCalendarDelegate(desired),
	}
	resource := &calendarDelegateResource{client: client}

	if err := resource.rollbackCreatedIfAllowed(
		t.Context(),
		desired,
		false,
	); err != nil {
		t.Fatalf("rollbackCreatedIfAllowed() error: %v", err)
	}
	if client.removeCalls != 0 {
		t.Fatalf(
			"rollbackCreatedIfAllowed() made %d remove calls, want 0",
			client.removeCalls,
		)
	}
	if client.delegate == nil {
		t.Fatal("rollbackCreatedIfAllowed() removed the existing delegate")
	}
}

func TestCalendarDelegateRollbackCreatedIsOwnershipBounded(t *testing.T) {
	t.Parallel()

	desired := testCalendarDelegateDefinition()

	t.Run("matching delegate is removed", func(t *testing.T) {
		t.Parallel()

		client := &fakeCalendarDelegateClient{
			delegate: testCalendarDelegate(desired),
		}
		resource := &calendarDelegateResource{client: client}

		if err := resource.rollbackCreated(t.Context(), desired); err != nil {
			t.Fatalf("rollbackCreated() error: %v", err)
		}
		if client.delegate != nil {
			t.Fatalf("rollbackCreated() left delegate %#v", client.delegate)
		}
	})

	t.Run("changed delegate is preserved", func(t *testing.T) {
		t.Parallel()

		changed := desired
		changed.ReadOnly = false
		client := &fakeCalendarDelegateClient{
			delegate: testCalendarDelegate(changed),
		}
		resource := &calendarDelegateResource{client: client}

		if err := resource.rollbackCreated(t.Context(), desired); err == nil {
			t.Fatal("rollbackCreated() returned no error")
		}
		if client.delegate == nil || !calendarDelegatesEqual(*client.delegate, changed) {
			t.Fatalf("rollbackCreated() changed delegate to %#v", client.delegate)
		}
	})
}

func TestCalendarDelegateRollbackUpdatedRestoresOnlyAttemptedState(t *testing.T) {
	t.Parallel()

	previous := testCalendarDelegateDefinition()
	attempted := previous
	attempted.ReadOnly = false

	t.Run("previous state needs no mutation", func(t *testing.T) {
		t.Parallel()

		client := &fakeCalendarDelegateClient{
			delegate: testCalendarDelegate(previous),
		}
		resource := &calendarDelegateResource{client: client}

		if err := resource.rollbackUpdated(
			t.Context(),
			previous,
			attempted,
		); err != nil {
			t.Fatalf("rollbackUpdated() error: %v", err)
		}
		if client.updateCalls != 0 {
			t.Fatalf("rollbackUpdated() made %d update calls", client.updateCalls)
		}
	})

	t.Run("attempted state is restored", func(t *testing.T) {
		t.Parallel()

		client := &fakeCalendarDelegateClient{
			delegate: testCalendarDelegate(attempted),
		}
		resource := &calendarDelegateResource{client: client}

		if err := resource.rollbackUpdated(
			t.Context(),
			previous,
			attempted,
		); err != nil {
			t.Fatalf("rollbackUpdated() error: %v", err)
		}
		if client.delegate == nil ||
			!calendarDelegatesEqual(*client.delegate, previous) {
			t.Fatalf("rollbackUpdated() left delegate %#v", client.delegate)
		}
	})

	t.Run("third party state is preserved", func(t *testing.T) {
		t.Parallel()

		changed := attempted
		changed.Delegatee = "other@example.test"
		client := &fakeCalendarDelegateClient{
			delegate: testCalendarDelegate(changed),
		}
		resource := &calendarDelegateResource{client: client}

		if err := resource.rollbackUpdated(
			t.Context(),
			previous,
			attempted,
		); err == nil {
			t.Fatal("rollbackUpdated() returned no error")
		}
		if client.updateCalls != 0 {
			t.Fatalf("rollbackUpdated() made %d update calls", client.updateCalls)
		}
	})
}

func testCalendarDelegateDefinition() cpanelcalendar.Definition {
	return cpanelcalendar.Definition{
		Delegator: "owner@example.test",
		Delegatee: "delegate@example.test",
		Calendar:  cpanelcalendar.DefaultCalendar,
		ReadOnly:  true,
	}
}

func testCalendarDelegate(
	definition cpanelcalendar.Definition,
) *cpanelcalendar.Delegate {
	return &cpanelcalendar.Delegate{
		Delegator:    definition.Delegator,
		Delegatee:    definition.Delegatee,
		Calendar:     definition.Calendar,
		CalendarName: "cPanel CalDAV Calendar",
		ReadOnly:     definition.ReadOnly,
	}
}

type fakeCalendarDelegateClient struct {
	delegate *cpanelcalendar.Delegate

	addErr     error
	addMutates bool
	updateErr  error
	removeErr  error

	updateCalls int
	removeCalls int
}

func (c *fakeCalendarDelegateClient) AddDelegate(
	_ context.Context,
	definition cpanelcalendar.Definition,
) error {
	if c.addErr == nil || c.addMutates {
		c.delegate = testCalendarDelegate(definition)
	}

	return c.addErr
}

func (c *fakeCalendarDelegateClient) CalendarExists(
	context.Context,
	string,
	string,
) (bool, error) {
	return true, nil
}

func (c *fakeCalendarDelegateClient) GetDelegate(
	_ context.Context,
	delegator string,
	calendar string,
	delegatee string,
) (*cpanelcalendar.Delegate, error) {
	if c.delegate == nil ||
		c.delegate.Delegator != delegator ||
		c.delegate.Calendar != calendar ||
		c.delegate.Delegatee != delegatee {
		return nil, nil
	}

	result := *c.delegate

	return &result, nil
}

func (c *fakeCalendarDelegateClient) LockDelegate(
	string,
	string,
	string,
) func() {
	return func() {}
}

func (c *fakeCalendarDelegateClient) RemoveDelegate(
	context.Context,
	string,
	string,
	string,
) error {
	c.removeCalls++
	if c.removeErr == nil {
		c.delegate = nil
	}

	return c.removeErr
}

func (c *fakeCalendarDelegateClient) UpdateDelegate(
	_ context.Context,
	definition cpanelcalendar.Definition,
) error {
	c.updateCalls++
	if c.updateErr == nil {
		c.delegate = testCalendarDelegate(definition)
	}

	return c.updateErr
}
