package provider

import (
	"context"
	"errors"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanellogmanager "terraform-provider-cpanel/internal/cpanel/logmanager"
)

func TestLogSettingsResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewLogSettingsResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, name := range []string{"archive_logs", "prune_archives"} {
		attribute, ok := response.Schema.Attributes[name].(resourceschema.BoolAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required bool", name)
		}
	}
	retention, ok := response.Schema.Attributes["retention_days"].(resourceschema.Int64Attribute)
	if !ok || !retention.Required {
		t.Fatal("retention_days must be a required int64")
	}
	for _, name := range []string{
		"account",
		"effective_retention_days",
		"using_default_retention",
		"restore_archive_logs",
		"restore_prune_archives",
		"restore_retention_days",
	} {
		if !response.Schema.Attributes[name].IsComputed() {
			t.Fatalf("%s must be computed", name)
		}
	}
}

func TestLogSettingsTransitionRestoresAfterMutationFailure(t *testing.T) {
	t.Parallel()

	original := cpanellogmanager.Settings{
		ArchiveLogs:   true,
		PruneArchive:  true,
		RetentionDays: 30,
		UsingDefault:  true,
	}
	target := cpanellogmanager.Definition{
		ArchiveLogs:   false,
		PruneArchive:  false,
		RetentionDays: 7,
	}
	client := &fakeLogSettingsClient{
		current:    original,
		setErr:     errors.New("connection closed after request"),
		errMutates: true,
	}
	resource := &logSettingsResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 2 {
		t.Fatalf("Set() call count = %d, want 2", len(client.setCalls))
	}
	if !cpanellogmanager.SettingsMatchDefinition(
		client.current,
		original.Definition(),
	) {
		t.Fatalf(
			"transition() left settings %#v, want %#v",
			client.current,
			original,
		)
	}
}

func TestLogSettingsTransitionSkipsRollbackAfterRejectedMutation(t *testing.T) {
	t.Parallel()

	original := cpanellogmanager.Settings{
		ArchiveLogs:   true,
		PruneArchive:  true,
		RetentionDays: 30,
		UsingDefault:  true,
	}
	target := cpanellogmanager.Definition{
		ArchiveLogs:   false,
		PruneArchive:  false,
		RetentionDays: 7,
	}
	client := &fakeLogSettingsClient{
		current: original,
		setErr: &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "LogManager",
			Function: "set_settings",
			Messages: []string{"rejected"},
		},
	}
	resource := &logSettingsResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf("Set() call count = %d, want 1", len(client.setCalls))
	}
	if client.current != original {
		t.Fatalf(
			"transition() changed settings to %#v, want %#v",
			client.current,
			original,
		)
	}
}

func TestLogSettingsTransitionPreservesConcurrentChange(t *testing.T) {
	t.Parallel()

	original := cpanellogmanager.Settings{
		ArchiveLogs:   true,
		PruneArchive:  true,
		RetentionDays: 30,
		UsingDefault:  true,
	}
	target := cpanellogmanager.Definition{
		ArchiveLogs:   false,
		PruneArchive:  false,
		RetentionDays: 7,
	}
	concurrent := cpanellogmanager.Settings{
		ArchiveLogs:   false,
		PruneArchive:  true,
		RetentionDays: 14,
		UsingDefault:  false,
	}
	client := &fakeLogSettingsClient{
		current:         original,
		setErr:          errors.New("connection closed after request"),
		stateAfterError: &concurrent,
	}
	resource := &logSettingsResource{client: client}

	if _, err := resource.transition(
		t.Context(),
		original,
		target,
	); err == nil {
		t.Fatal("transition() returned no error")
	}
	if len(client.setCalls) != 1 {
		t.Fatalf("Set() call count = %d, want 1", len(client.setCalls))
	}
	if client.current != concurrent {
		t.Fatalf(
			"transition() changed concurrent settings to %#v, want %#v",
			client.current,
			concurrent,
		)
	}
}

func TestLogSettingsRestoreSkipsMatchingState(t *testing.T) {
	t.Parallel()

	current := cpanellogmanager.Settings{
		ArchiveLogs:   true,
		PruneArchive:  false,
		RetentionDays: 0,
		UsingDefault:  false,
	}
	client := &fakeLogSettingsClient{current: current}
	resource := &logSettingsResource{client: client}

	if err := resource.restore(
		t.Context(),
		current.Definition(),
	); err != nil {
		t.Fatalf("restore() error: %v", err)
	}
	if len(client.setCalls) != 0 {
		t.Fatalf("Set() call count = %d, want 0", len(client.setCalls))
	}
}

func TestLogSettingsGuardedRestorePreservesConcurrentChange(t *testing.T) {
	t.Parallel()

	original := cpanellogmanager.Definition{
		ArchiveLogs:   true,
		PruneArchive:  true,
		RetentionDays: -1,
	}
	managed := cpanellogmanager.Definition{
		ArchiveLogs:   false,
		PruneArchive:  false,
		RetentionDays: 7,
	}
	concurrent := cpanellogmanager.Settings{
		ArchiveLogs:   false,
		PruneArchive:  true,
		RetentionDays: 14,
		UsingDefault:  false,
	}
	client := &fakeLogSettingsClient{current: concurrent}
	resource := &logSettingsResource{client: client}

	if err := resource.restoreIfCurrentMatches(
		t.Context(),
		managed,
		original,
	); err == nil {
		t.Fatal("restoreIfCurrentMatches() returned no error")
	}
	if len(client.setCalls) != 0 {
		t.Fatalf("Set() call count = %d, want 0", len(client.setCalls))
	}
	if client.current != concurrent {
		t.Fatalf(
			"guarded restore changed settings to %#v, want %#v",
			client.current,
			concurrent,
		)
	}
}

type fakeLogSettingsClient struct {
	current cpanellogmanager.Settings

	setErr     error
	errMutates bool
	setCalls   []cpanellogmanager.Definition

	stateAfterError *cpanellogmanager.Settings
}

func (c *fakeLogSettingsClient) Get(
	context.Context,
) (*cpanellogmanager.Settings, error) {
	result := c.current

	return &result, nil
}

func (c *fakeLogSettingsClient) Set(
	_ context.Context,
	definition cpanellogmanager.Definition,
) (*cpanellogmanager.Settings, error) {
	c.setCalls = append(c.setCalls, definition)
	if c.setErr != nil {
		err := c.setErr
		c.setErr = nil
		if c.errMutates {
			c.current = fakeLogSettingsFromDefinition(
				c.current,
				definition,
			)
		} else if c.stateAfterError != nil {
			c.current = *c.stateAfterError
		}

		return nil, err
	}

	c.current = fakeLogSettingsFromDefinition(c.current, definition)
	result := c.current

	return &result, nil
}

func fakeLogSettingsFromDefinition(
	current cpanellogmanager.Settings,
	definition cpanellogmanager.Definition,
) cpanellogmanager.Settings {
	current.ArchiveLogs = definition.ArchiveLogs
	current.PruneArchive = definition.PruneArchive
	current.UsingDefault = definition.RetentionDays == -1
	if definition.RetentionDays >= 0 {
		current.RetentionDays = definition.RetentionDays
	}

	return current
}
