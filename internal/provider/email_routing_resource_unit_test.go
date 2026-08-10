package provider

import (
	"context"
	"errors"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailRoutingResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewEmailRoutingResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	domain, ok := response.Schema.Attributes["domain"].(resourceschema.StringAttribute)
	if !ok || !domain.Required || len(domain.PlanModifiers) == 0 {
		t.Fatal("domain must be a required replacement string")
	}
	mode, ok := response.Schema.Attributes["mode"].(resourceschema.StringAttribute)
	if !ok || !mode.Required {
		t.Fatal("mode must be a required string")
	}
	for _, name := range []string{
		"detected_mode",
		"primary_exchanger",
		"restore_mode",
	} {
		if !response.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestEmailRoutingModelsUseNullForMissingPrimaryExchanger(t *testing.T) {
	t.Parallel()

	routing := testEmailRouting(cpanelmail.RoutingModeRemote)
	routing.PrimaryExchanger = nil

	resourceModel := EmailRoutingResourceModel{}
	applyEmailRoutingToResourceModel(&resourceModel, routing)
	if !resourceModel.PrimaryExchanger.IsNull() {
		t.Fatalf(
			"resource primary exchanger = %#v, want null",
			resourceModel.PrimaryExchanger,
		)
	}

	dataSourceModel := emailRoutingToDataSourceModel(routing)
	if !dataSourceModel.PrimaryExchanger.IsNull() {
		t.Fatalf(
			"data source primary exchanger = %#v, want null",
			dataSourceModel.PrimaryExchanger,
		)
	}
}

func TestEmailRoutingTransitionRestoresAfterMutationFailure(t *testing.T) {
	t.Parallel()

	original := testEmailRouting(cpanelmail.RoutingModeAuto)
	client := &fakeEmailRoutingClient{
		current:    &original,
		setErr:     errors.New("connection closed after request"),
		errMutates: true,
	}
	resource := &emailRoutingResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		cpanelmail.RoutingModeRemote,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 2 {
		t.Fatalf("SetRouting() call count = %d, want 2", len(client.setCalls))
	}
	if client.current == nil ||
		client.current.Mode != cpanelmail.RoutingModeAuto {
		t.Fatalf("current routing = %#v, want auto", client.current)
	}
}

func TestEmailRoutingTransitionSkipsRollbackAfterRejectedMutation(
	t *testing.T,
) {
	t.Parallel()

	original := testEmailRouting(cpanelmail.RoutingModeAuto)
	client := &fakeEmailRoutingClient{
		current: &original,
		setErr: &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "Email",
			Function: "set_always_accept",
			Messages: []string{"rejected"},
		},
	}
	resource := &emailRoutingResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		cpanelmail.RoutingModeRemote,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf("SetRouting() call count = %d, want 1", len(client.setCalls))
	}
	if client.current == nil || client.current.Mode != original.Mode {
		t.Fatalf("current routing = %#v, want %#v", client.current, original)
	}
}

func TestEmailRoutingTransitionPreservesConcurrentChange(t *testing.T) {
	t.Parallel()

	original := testEmailRouting(cpanelmail.RoutingModeAuto)
	concurrent := testEmailRouting(cpanelmail.RoutingModeBackup)
	client := &fakeEmailRoutingClient{
		current:         &original,
		setErr:          errors.New("connection closed after request"),
		stateAfterError: &concurrent,
	}
	resource := &emailRoutingResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		cpanelmail.RoutingModeRemote,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf("SetRouting() call count = %d, want 1", len(client.setCalls))
	}
	if client.current == nil ||
		client.current.Mode != cpanelmail.RoutingModeBackup {
		t.Fatalf("current routing = %#v, want backup", client.current)
	}
}

func TestEmailRoutingGuardedRestorePreservesConcurrentChange(t *testing.T) {
	t.Parallel()

	concurrent := testEmailRouting(cpanelmail.RoutingModeBackup)
	client := &fakeEmailRoutingClient{current: &concurrent}
	resource := &emailRoutingResource{client: client}

	if err := resource.restoreIfCurrentMatches(
		t.Context(),
		concurrent.Domain,
		cpanelmail.RoutingModeRemote,
		cpanelmail.RoutingModeAuto,
	); err == nil {
		t.Fatal("restoreIfCurrentMatches() returned no error")
	}
	if len(client.setCalls) != 0 {
		t.Fatalf("SetRouting() call count = %d, want 0", len(client.setCalls))
	}
	if client.current == nil ||
		client.current.Mode != cpanelmail.RoutingModeBackup {
		t.Fatalf("current routing = %#v, want backup", client.current)
	}
}

func TestEmailRoutingRestoreSkipsMissingDomain(t *testing.T) {
	t.Parallel()

	client := &fakeEmailRoutingClient{}
	resource := &emailRoutingResource{client: client}

	if _, err := resource.restore(
		t.Context(),
		"missing.example.test",
		cpanelmail.RoutingModeAuto,
	); err != nil {
		t.Fatalf("restore() error: %v", err)
	}
	if len(client.setCalls) != 0 {
		t.Fatalf("SetRouting() call count = %d, want 0", len(client.setCalls))
	}
}

type fakeEmailRoutingClient struct {
	current *cpanelmail.Routing

	setErr     error
	errMutates bool
	setCalls   []cpanelmail.RoutingDefinition

	stateAfterError *cpanelmail.Routing
}

func (c *fakeEmailRoutingClient) GetRouting(
	context.Context,
	string,
) (*cpanelmail.Routing, error) {
	if c.current == nil {
		return nil, nil
	}
	result := *c.current

	return &result, nil
}

func (c *fakeEmailRoutingClient) SetRouting(
	_ context.Context,
	definition cpanelmail.RoutingDefinition,
) (*cpanelmail.Routing, []string, error) {
	c.setCalls = append(c.setCalls, definition)
	if c.setErr != nil {
		err := c.setErr
		c.setErr = nil
		if c.errMutates {
			current := testEmailRouting(definition.Mode)
			current.Domain = definition.Domain
			c.current = &current
		} else if c.stateAfterError != nil {
			current := *c.stateAfterError
			c.current = &current
		}

		return nil, nil, err
	}

	current := testEmailRouting(definition.Mode)
	current.Domain = definition.Domain
	c.current = &current
	result := current

	return &result, nil, nil
}

func (c *fakeEmailRoutingClient) LockRoutingDomain(string) func() {
	return func() {}
}

func testEmailRouting(mode cpanelmail.RoutingMode) cpanelmail.Routing {
	detected := mode
	if mode == cpanelmail.RoutingModeAuto {
		detected = cpanelmail.RoutingModeLocal
	}

	exchanger := "mail.routing.example.test"

	return cpanelmail.Routing{
		Domain:           "routing.example.test",
		Mode:             mode,
		DetectedMode:     detected,
		PrimaryExchanger: &exchanger,
		Local:            detected == cpanelmail.RoutingModeLocal,
		Remote:           detected == cpanelmail.RoutingModeRemote,
		Backup:           detected == cpanelmail.RoutingModeBackup,
	}
}
