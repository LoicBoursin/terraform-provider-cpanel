package provider

import "testing"

func TestValidateEmailForwarderDestination(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		address   string
		wantError bool
	}{
		"valid":            {address: "terraform+cpanel@example.test"},
		"missing at":       {address: "terraform.example.test", wantError: true},
		"multiple at":      {address: "terraform@@example.test", wantError: true},
		"multiple targets": {address: "one@example.test,two@example.test", wantError: true},
		"pipe":             {address: "|script@example.test", wantError: true},
		"whitespace":       {address: "terraform @example.test", wantError: true},
		"uppercase domain": {address: "terraform@Example.test", wantError: true},
		"invalid domain":   {address: "terraform@-example.test", wantError: true},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateEmailForwarderDestination(testCase.address)
			if testCase.wantError && err == nil {
				t.Fatalf(
					"validateEmailForwarderDestination(%q) returned no error",
					testCase.address,
				)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"validateEmailForwarderDestination(%q) error: %v",
					testCase.address,
					err,
				)
			}
		})
	}
}
