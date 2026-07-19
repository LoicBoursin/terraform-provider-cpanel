package provider

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
)

func TestSensitiveMutationErrorRedactsRemoteDetails(t *testing.T) {
	t.Parallel()

	const secret = "secret-value-that-must-not-leak"

	tests := map[string]struct {
		err      error
		wantText string
	}{
		"API error": {
			err: &cpanelapi.APIError{
				API:      "UAPI",
				Module:   "Module",
				Function: "function",
				Messages: []string{"rejected password " + secret},
			},
			wantText: "cPanel rejected the password update request",
		},
		"wrapped HTTP error": {
			err: fmt.Errorf(
				"request containing %s: %w",
				secret,
				&cpanelapi.HTTPError{StatusCode: 503},
			),
			wantText: "cPanel returned HTTP status 503 for the password update request",
		},
		"ambiguous error": {
			err:      errors.New("transport failed after sending " + secret),
			wantText: "the cPanel password update request failed or returned an ambiguous response",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := sensitiveMutationError(test.err, "password update")
			if got == nil {
				t.Fatal("sensitiveMutationError() = nil, want error")
			}
			if got.Error() != test.wantText {
				t.Fatalf(
					"sensitiveMutationError() = %q, want %q",
					got,
					test.wantText,
				)
			}
			if strings.Contains(got.Error(), secret) {
				t.Fatalf("sensitiveMutationError() leaked %q", secret)
			}
		})
	}
}

func TestSensitiveMutationErrorPreservesNil(t *testing.T) {
	t.Parallel()

	if err := sensitiveMutationError(nil, "password update"); err != nil {
		t.Fatalf("sensitiveMutationError(nil) = %v, want nil", err)
	}
}

func TestCreateMutationErrorDetailOnlyOffersRecoveryForAmbiguousErrors(
	t *testing.T,
) {
	t.Parallel()

	deterministic := createMutationErrorDetail(
		&cpanelapi.APIError{
			API:      "UAPI",
			Module:   "Module",
			Function: "function",
			Messages: []string{"rejected"},
		},
		"resource creation",
		"the resource",
	)
	if strings.Contains(deterministic, "import") ||
		strings.Contains(deterministic, "may have applied") {
		t.Fatalf("deterministic detail offers ambiguous recovery: %q", deterministic)
	}

	ambiguous := createMutationErrorDetail(
		errors.New("connection closed"),
		"resource creation",
		"the resource",
	)
	if !strings.Contains(ambiguous, "may have applied") ||
		!strings.Contains(ambiguous, "import") {
		t.Fatalf("ambiguous detail does not offer recovery: %q", ambiguous)
	}
}
