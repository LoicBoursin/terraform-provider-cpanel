package provider

import "testing"

func TestValidateRedirectSource(t *testing.T) {
	t.Parallel()

	testCases := map[string]bool{
		"/":                  true,
		"/old/path":          true,
		"/encoded%20space":   true,
		"":                   false,
		"relative":           false,
		"//example.com/path": false,
		"/path?query=1":      false,
		"/path#fragment":     false,
		"/path|other":        false,
		"/space here":        false,
	}

	for value, expectedValid := range testCases {
		err := validateRedirectSource(value)
		if got := err == nil; got != expectedValid {
			t.Errorf(
				"validateRedirectSource(%q) validity = %t, want %t; error: %v",
				value,
				got,
				expectedValid,
				err,
			)
		}
	}
}

func TestValidateRedirectDestination(t *testing.T) {
	t.Parallel()

	testCases := map[string]bool{
		"https://example.net/new":        true,
		"http://example.net/new?q=1":     true,
		"https://example.net/new%20path": true,
		"https://example.net/new path":   false,
		"/relative":                      false,
		"ftp://example.net/file":         false,
		"https://":                       false,
		"https://user:pass@example.net":  false,
		"":                               false,
	}

	for value, expectedValid := range testCases {
		err := validateRedirectDestination(value)
		if got := err == nil; got != expectedValid {
			t.Errorf(
				"validateRedirectDestination(%q) validity = %t, want %t; error: %v",
				value,
				got,
				expectedValid,
				err,
			)
		}
	}
}
