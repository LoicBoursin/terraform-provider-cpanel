package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateEmailAutoResponder(t *testing.T) {
	t.Parallel()

	valid := EmailAutoResponderModel{
		From:      types.StringValue("Terraform"),
		Subject:   types.StringValue("Away"),
		Body:      types.StringValue("Back soon"),
		Charset:   types.StringValue("UTF-8"),
		StartUnix: types.Int64Value(0),
		StopUnix:  types.Int64Value(0),
	}
	if err := validateEmailAutoResponder(valid); err != nil {
		t.Fatalf("validateEmailAutoResponder() error: %v", err)
	}

	invalidSchedule := valid
	invalidSchedule.StartUnix = types.Int64Value(200)
	invalidSchedule.StopUnix = types.Int64Value(100)
	if err := validateEmailAutoResponder(invalidSchedule); err == nil {
		t.Fatal("validateEmailAutoResponder() accepted stop before start")
	}

	invalidCharset := valid
	invalidCharset.Charset = types.StringValue("UTF 8")
	if err := validateEmailAutoResponder(invalidCharset); err == nil {
		t.Fatal("validateEmailAutoResponder() accepted invalid charset")
	}
}

func TestNormalizeAutoResponderBody(t *testing.T) {
	t.Parallel()

	testCases := map[string]string{
		"plain":        "plain",
		"newline":      "plain",
		"windows":      "plain",
		"two newlines": "plain\n",
	}

	inputs := map[string]string{
		"plain":        "plain",
		"newline":      "plain\n",
		"windows":      "plain\r\n",
		"two newlines": "plain\n\n",
	}

	for name, input := range inputs {
		name := name
		input := input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeAutoResponderBody(input); got != testCases[name] {
				t.Fatalf(
					"normalizeAutoResponderBody(%q) = %q, want %q",
					input,
					got,
					testCases[name],
				)
			}
		})
	}
}
