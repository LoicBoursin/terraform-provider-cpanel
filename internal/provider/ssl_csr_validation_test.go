package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSSLCSROptionalSubjectValidators(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value     types.String
		wantValid bool
	}{
		"omitted": {
			value:     types.StringNull(),
			wantValid: true,
		},
		"unknown": {
			value:     types.StringUnknown(),
			wantValid: true,
		},
		"present": {
			value:     types.StringValue("Platform Engineering"),
			wantValid: true,
		},
		"explicitly empty": {
			value: types.StringValue(""),
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := validateSSLCSRString(
				testCase.value,
				"organizational_unit_name",
				sslCSROptionalSubjectValidators(),
			)
			if got := !response.Diagnostics.HasError(); got != testCase.wantValid {
				t.Fatalf(
					"sslCSROptionalSubjectValidators() validity = %t, want %t; diagnostics: %v",
					got,
					testCase.wantValid,
					response.Diagnostics,
				)
			}
		})
	}
}

func TestSSLCSREmailValidators(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value     types.String
		wantValid bool
	}{
		"omitted": {
			value:     types.StringNull(),
			wantValid: true,
		},
		"unknown": {
			value:     types.StringUnknown(),
			wantValid: true,
		},
		"present": {
			value:     types.StringValue("admin@example.test"),
			wantValid: true,
		},
		"explicitly empty": {
			value: types.StringValue(""),
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := validateSSLCSRString(
				testCase.value,
				"email_address",
				sslCSREmailValidators(),
			)
			if got := !response.Diagnostics.HasError(); got != testCase.wantValid {
				t.Fatalf(
					"sslCSREmailValidators() validity = %t, want %t; diagnostics: %v",
					got,
					testCase.wantValid,
					response.Diagnostics,
				)
			}
		})
	}
}

func TestSSLCSRDomainsValidatorDefersUnknownElements(t *testing.T) {
	t.Parallel()

	response := validateSSLCSRDomains(
		types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("example.test"),
			types.StringUnknown(),
		}),
	)
	if response.Diagnostics.HasError() {
		t.Fatalf(
			"sslCSRDomainsValidator returned diagnostics for an unknown element: %v",
			response.Diagnostics,
		)
	}
}

func TestSSLCSRDomainsValidatorRejectsNullElements(t *testing.T) {
	t.Parallel()

	testCases := map[string][]attr.Value{
		"null": {
			types.StringNull(),
		},
		"unknown before null": {
			types.StringUnknown(),
			types.StringNull(),
		},
	}

	for name, elements := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			response := validateSSLCSRDomains(
				types.ListValueMust(types.StringType, elements),
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("sslCSRDomainsValidator accepted a null element")
			}
		})
	}
}

func validateSSLCSRString(
	value types.String,
	attributeName string,
	validators []validator.String,
) *validator.StringResponse {
	request := validator.StringRequest{
		ConfigValue: value,
		Path:        path.Root(attributeName),
	}
	response := &validator.StringResponse{}
	for _, stringValidator := range validators {
		stringValidator.ValidateString(context.Background(), request, response)
	}

	return response
}

func validateSSLCSRDomains(value types.List) *validator.ListResponse {
	request := validator.ListRequest{
		ConfigValue: value,
		Path:        path.Root("domains"),
	}
	response := &validator.ListResponse{}
	sslCSRDomainsValidator{}.ValidateList(
		context.Background(),
		request,
		response,
	)

	return response
}
