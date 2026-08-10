package boxtrapper

import (
	"math"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSettingsDefinitionAndMatchIgnoreObservedFromName(t *testing.T) {
	t.Parallel()

	fromName := "Observed Sender"
	settings := Settings{
		Account: "box@example.test",
		Enabled: true,
		Configuration: Configuration{
			EnableAutoWhitelist:    true,
			FromAddresses:          "box@example.test",
			FromName:               &fromName,
			QueueDays:              15,
			SpamScore:              -2.5,
			WhitelistByAssociation: true,
		},
	}
	definition := settings.Definition()
	if definition != (Definition{
		Enabled:                true,
		EnableAutoWhitelist:    true,
		FromAddresses:          "box@example.test",
		QueueDays:              15,
		SpamScore:              -2.5,
		WhitelistByAssociation: true,
	}) {
		t.Fatalf("Definition() = %#v", definition)
	}

	emptyFromName := ""
	settings.FromName = &emptyFromName
	if !SettingsMatchDefinition(settings, definition) {
		t.Fatal("SettingsMatchDefinition() considered FromName")
	}

	settings.Enabled = false
	if SettingsMatchDefinition(settings, definition) {
		t.Fatal("SettingsMatchDefinition() ignored Enabled")
	}
	settings.Enabled = true
	settings.QueueDays = 9
	if SettingsMatchDefinition(settings, definition) {
		t.Fatal("SettingsMatchDefinition() ignored QueueDays")
	}
	settings.QueueDays = definition.QueueDays
	definition.SpamScore = 3.7000000000000002
	settings.SpamScore = 3.7
	if !SettingsMatchDefinition(settings, definition) {
		t.Fatal("SettingsMatchDefinition() rejected floating-point noise")
	}
}

func TestValidateDefinition(t *testing.T) {
	t.Parallel()

	valid := Definition{
		Enabled:                false,
		EnableAutoWhitelist:    true,
		FromAddresses:          "box@example.test",
		QueueDays:              15,
		SpamScore:              -2.5,
		WhitelistByAssociation: true,
	}
	if err := ValidateDefinition(valid); err != nil {
		t.Fatalf("ValidateDefinition() error: %v", err)
	}

	roundedFloat := valid
	roundedFloat.SpamScore = 3.7000000000000002
	if err := ValidateDefinition(roundedFloat); err != nil {
		t.Fatalf("ValidateDefinition() rejected floating-point noise: %v", err)
	}

	tests := map[string]struct {
		mutate  func(*Definition)
		wantErr string
	}{
		"queue below one": {
			mutate: func(value *Definition) {
				value.QueueDays = 0
			},
			wantErr: "at least 1",
		},
		"NaN spam score": {
			mutate: func(value *Definition) {
				value.SpamScore = math.NaN()
			},
			wantErr: "finite",
		},
		"infinite spam score": {
			mutate: func(value *Definition) {
				value.SpamScore = math.Inf(1)
			},
			wantErr: "finite",
		},
		"spam score precision above one decimal": {
			mutate: func(value *Definition) {
				value.SpamScore = 3.75
			},
			wantErr: "one decimal place",
		},
		"spam score precision hidden below fixed decimal tolerance": {
			mutate: func(value *Definition) {
				value.SpamScore = 3.7000000001
			},
			wantErr: "one decimal place",
		},
		"from addresses surrounding whitespace": {
			mutate: func(value *Definition) {
				value.FromAddresses = " box@example.test"
			},
			wantErr: "surrounding whitespace",
		},
		"from addresses control character": {
			mutate: func(value *Definition) {
				value.FromAddresses = "box@example.test\nother@example.test"
			},
			wantErr: "control characters",
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			definition := valid
			test.mutate(&definition)
			err := ValidateDefinition(definition)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf(
					"ValidateDefinition() error = %v, want containing %q",
					err,
					test.wantErr,
				)
			}
		})
	}
}

func TestClientRejectsInvalidInputsBeforeRequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	client, closeServer := newCountingTestClient(t, &requests)
	defer closeServer()

	for _, account := range []string{
		"",
		" box@example.test",
		"box@example.test\t",
		"box@example.test\nother",
	} {
		if _, err := client.Get(t.Context(), account); err == nil {
			t.Errorf("Get(%q) returned no error", account)
		}
		if _, _, err := client.SetStatus(
			t.Context(),
			account,
			true,
		); err == nil {
			t.Errorf("SetStatus(%q) returned no error", account)
		}
	}

	invalidDefinition := Definition{
		FromAddresses: "box@example.test",
		QueueDays:     15,
		SpamScore:     3.75,
	}
	fromName := "Sender"
	if _, _, err := client.SaveConfiguration(
		t.Context(),
		"box@example.test",
		invalidDefinition,
		&fromName,
	); err == nil {
		t.Fatal("SaveConfiguration() returned no precision error")
	}

	validDefinition := invalidDefinition
	validDefinition.SpamScore = 3.7
	if _, _, err := client.SaveConfiguration(
		t.Context(),
		"box@example.test",
		validDefinition,
		nil,
	); err == nil || !strings.Contains(err.Error(), "from_name as null") {
		t.Fatalf(
			"SaveConfiguration() null from_name error = %v",
			err,
		)
	}

	if requests.Load() != 0 {
		t.Fatalf("request count = %d, want 0", requests.Load())
	}
}
