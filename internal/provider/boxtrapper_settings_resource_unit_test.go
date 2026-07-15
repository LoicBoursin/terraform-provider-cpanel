package provider

import (
	"context"
	"errors"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelboxtrapper "terraform-provider-cpanel/internal/cpanel/boxtrapper"
)

func TestBoxTrapperSettingsResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewBoxTrapperSettingsResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	account, ok := response.Schema.Attributes["account"].(resourceschema.StringAttribute)
	if !ok || !account.Required || len(account.PlanModifiers) == 0 {
		t.Fatal("account must be a required replacement string")
	}
	enabled, ok := response.Schema.Attributes["enabled"].(resourceschema.BoolAttribute)
	if !ok || !enabled.Required {
		t.Fatal("enabled must be a required bool")
	}
	for _, name := range []string{
		"enable_auto_whitelist",
		"from_addresses",
		"queue_days",
		"spam_score",
		"whitelist_by_association",
	} {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsOptional() || !attribute.IsComputed() {
			t.Fatalf("%s must be optional and computed", name)
		}
	}
	for _, name := range []string{
		"from_name",
		"restore_enabled",
		"restore_enable_auto_whitelist",
		"restore_from_addresses",
		"restore_queue_days",
		"restore_spam_score",
		"restore_whitelist_by_association",
	} {
		if !response.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestBoxTrapperDefinitionUsesCurrentValuesForOmittedSettings(
	t *testing.T,
) {
	t.Parallel()

	current := testBoxTrapperSettings()
	model := BoxTrapperSettingsResourceModel{
		Enabled:                types.BoolValue(true),
		EnableAutoWhitelist:    types.BoolUnknown(),
		FromAddresses:          types.StringNull(),
		QueueDays:              types.Int64Unknown(),
		SpamScore:              types.Float64Null(),
		WhitelistByAssociation: types.BoolUnknown(),
	}

	definition := boxTrapperDefinitionFromResourceModel(model, current)
	if !definition.Enabled {
		t.Fatal("enabled = false, want true")
	}
	fallback := current.Definition()
	fallback.Enabled = true
	if definition != fallback {
		t.Fatalf("definition = %#v, want %#v", definition, fallback)
	}
}

func TestBoxTrapperModelsPreserveNullFromName(t *testing.T) {
	t.Parallel()

	settings := testBoxTrapperSettings()
	settings.FromName = nil

	resourceModel := BoxTrapperSettingsResourceModel{}
	applyBoxTrapperSettingsToResourceModel(&resourceModel, settings)
	if !resourceModel.FromName.IsNull() {
		t.Fatalf("resource from_name = %#v, want null", resourceModel.FromName)
	}

	dataSourceModel := boxTrapperSettingsToDataSourceModel(settings)
	if !dataSourceModel.FromName.IsNull() {
		t.Fatalf(
			"data source from_name = %#v, want null",
			dataSourceModel.FromName,
		)
	}
}

func TestBoxTrapperTransitionAppliesConfigurationBeforeStatus(
	t *testing.T,
) {
	t.Parallel()

	original := testBoxTrapperSettings()
	target := original.Definition()
	target.Enabled = true
	target.EnableAutoWhitelist = false
	target.QueueDays = 9
	target.SpamScore = 3.7
	client := &fakeBoxTrapperSettingsClient{
		current:      &original,
		saveWarnings: []string{"configuration warning"},
	}
	resource := &boxTrapperSettingsResource{client: client}

	updated, warnings, err := resource.transition(
		t.Context(),
		original,
		target,
	)
	if err != nil {
		t.Fatalf("transition() error: %v", err)
	}
	if len(client.calls) != 2 ||
		client.calls[0] != "save" ||
		client.calls[1] != "status" {
		t.Fatalf("mutation calls = %#v, want save then status", client.calls)
	}
	if len(warnings) != 1 || warnings[0] != "configuration warning" {
		t.Fatalf("warnings = %#v", warnings)
	}
	if !cpanelboxtrapper.SettingsMatchDefinition(*updated, target) {
		t.Fatalf("updated settings = %#v, want %#v", updated, target)
	}
}

func TestBoxTrapperTransitionAllowsStatusOnlyWithNullFromName(
	t *testing.T,
) {
	t.Parallel()

	original := testBoxTrapperSettings()
	original.FromName = nil
	target := original.Definition()
	target.Enabled = true
	client := &fakeBoxTrapperSettingsClient{current: &original}
	resource := &boxTrapperSettingsResource{client: client}

	updated, _, err := resource.transition(
		t.Context(),
		original,
		target,
	)
	if err != nil {
		t.Fatalf("transition() error: %v", err)
	}
	if len(client.calls) != 1 || client.calls[0] != "status" {
		t.Fatalf("mutation calls = %#v, want status only", client.calls)
	}
	if updated.FromName != nil {
		t.Fatalf("updated from_name = %#v, want null", updated.FromName)
	}
}

func TestBoxTrapperTransitionRollsBackAmbiguousConfigurationMutation(
	t *testing.T,
) {
	t.Parallel()

	original := testBoxTrapperSettings()
	target := original.Definition()
	target.QueueDays = 9
	client := &fakeBoxTrapperSettingsClient{
		current: &original,
		saveErr: &cpanelboxtrapper.MutationVerificationError{
			Operation: "configuration",
			Err: &cpanelapi.APIError{
				API:      "API 2",
				Module:   "BoxTrapper",
				Function: "accountmanagelist",
				Messages: []string{"reread failed"},
			},
		},
		saveErrMutates: true,
	}
	resource := &boxTrapperSettingsResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.calls) != 2 ||
		client.calls[0] != "save" ||
		client.calls[1] != "save" {
		t.Fatalf("mutation calls = %#v, want save then rollback save", client.calls)
	}
	if client.current == nil ||
		!boxTrapperSettingsMatchSettings(*client.current, original) {
		t.Fatalf("current settings = %#v, want %#v", client.current, original)
	}
}

func TestBoxTrapperTransitionRollsBackConfigurationAfterRejectedStatus(
	t *testing.T,
) {
	t.Parallel()

	original := testBoxTrapperSettings()
	target := original.Definition()
	target.Enabled = true
	target.QueueDays = 9
	client := &fakeBoxTrapperSettingsClient{
		current: &original,
		statusErr: &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "BoxTrapper",
			Function: "set_status",
			Messages: []string{"rejected"},
		},
	}
	resource := &boxTrapperSettingsResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.calls) != 3 ||
		client.calls[0] != "save" ||
		client.calls[1] != "status" ||
		client.calls[2] != "save" {
		t.Fatalf(
			"mutation calls = %#v, want save, status, rollback save",
			client.calls,
		)
	}
	if client.current == nil ||
		!boxTrapperSettingsMatchSettings(*client.current, original) {
		t.Fatalf("current settings = %#v, want %#v", client.current, original)
	}
}

func TestBoxTrapperTransitionPreservesConcurrentChange(t *testing.T) {
	t.Parallel()

	original := testBoxTrapperSettings()
	target := original.Definition()
	target.QueueDays = 9
	concurrent := original
	concurrent.QueueDays = 12
	client := &fakeBoxTrapperSettingsClient{
		current:             &original,
		saveErr:             errors.New("connection closed after request"),
		stateAfterSaveError: &concurrent,
	}
	resource := &boxTrapperSettingsResource{client: client}

	if _, _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.calls) != 1 {
		t.Fatalf("mutation calls = %#v, want one save", client.calls)
	}
	if client.current == nil ||
		!boxTrapperSettingsMatchSettings(*client.current, concurrent) {
		t.Fatalf(
			"current settings = %#v, want concurrent %#v",
			client.current,
			concurrent,
		)
	}
}

func TestBoxTrapperRestoreSkipsMissingAccount(t *testing.T) {
	t.Parallel()

	client := &fakeBoxTrapperSettingsClient{}
	resource := &boxTrapperSettingsResource{client: client}

	warnings, err := resource.restore(
		t.Context(),
		"missing@example.test",
		testBoxTrapperSettings().Definition(),
	)
	if err != nil {
		t.Fatalf("restore() error: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(client.calls) != 0 {
		t.Fatalf("mutation calls = %#v, want none", client.calls)
	}
}

func TestValidateBoxTrapperAccount(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		account string
		wantErr bool
	}{
		"system account":      {account: "cpanel_user"},
		"mail account":        {account: "mail@example.test"},
		"empty":               {account: "", wantErr: true},
		"surrounding space":   {account: " mail@example.test", wantErr: true},
		"control character":   {account: "mail\n@example.test", wantErr: true},
		"invalid system name": {account: "_system", wantErr: true},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateBoxTrapperAccount(test.account)
			if test.wantErr && err == nil {
				t.Fatal("validateBoxTrapperAccount() returned no error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("validateBoxTrapperAccount() error: %v", err)
			}
		})
	}
}

type fakeBoxTrapperSettingsClient struct {
	current *cpanelboxtrapper.Settings

	saveErr        error
	saveErrMutates bool
	statusErr      error

	stateAfterSaveError *cpanelboxtrapper.Settings

	saveWarnings   []string
	statusWarnings []string
	calls          []string
}

func (c *fakeBoxTrapperSettingsClient) Get(
	context.Context,
	string,
) (*cpanelboxtrapper.Settings, error) {
	if c.current == nil {
		return nil, nil
	}
	result := *c.current

	return &result, nil
}

func (c *fakeBoxTrapperSettingsClient) SaveConfiguration(
	_ context.Context,
	_ string,
	definition cpanelboxtrapper.Definition,
	fromName *string,
) (*cpanelboxtrapper.Settings, []string, error) {
	c.calls = append(c.calls, "save")
	if fromName == nil {
		return nil, c.saveWarnings, errors.New(
			"cannot save configuration with null from_name",
		)
	}
	if c.saveErr != nil {
		err := c.saveErr
		c.saveErr = nil
		if c.saveErrMutates && c.current != nil {
			updated := boxTrapperSettingsWithDefinition(
				*c.current,
				definition,
			)
			c.current = &updated
		} else if c.stateAfterSaveError != nil {
			updated := *c.stateAfterSaveError
			c.current = &updated
		}

		return nil, c.saveWarnings, err
	}
	if c.current == nil {
		return nil, c.saveWarnings, errors.New("missing account")
	}
	updated := boxTrapperSettingsWithDefinition(*c.current, definition)
	updated.Enabled = c.current.Enabled
	c.current = &updated
	result := updated

	return &result, c.saveWarnings, nil
}

func (c *fakeBoxTrapperSettingsClient) SetStatus(
	_ context.Context,
	_ string,
	enabled bool,
) (*cpanelboxtrapper.Settings, []string, error) {
	c.calls = append(c.calls, "status")
	if c.statusErr != nil {
		err := c.statusErr
		c.statusErr = nil

		return nil, c.statusWarnings, err
	}
	if c.current == nil {
		return nil, c.statusWarnings, errors.New("missing account")
	}
	updated := *c.current
	updated.Enabled = enabled
	c.current = &updated
	result := updated

	return &result, c.statusWarnings, nil
}

func (c *fakeBoxTrapperSettingsClient) LockAccount(string) func() {
	return func() {}
}

func testBoxTrapperSettings() cpanelboxtrapper.Settings {
	fromName := "Terraform Test Sender"
	settings := cpanelboxtrapper.Settings{
		Account: "mail@example.test",
		Enabled: false,
	}
	settings.EnableAutoWhitelist = true
	settings.FromAddresses = "mail@example.test"
	settings.FromName = &fromName
	settings.QueueDays = 15
	settings.SpamScore = -2.5
	settings.WhitelistByAssociation = true

	return settings
}
