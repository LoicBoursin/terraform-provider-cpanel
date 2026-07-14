package provider

import "testing"

func TestValidateMySQLRemoteHostNote(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		note      string
		wantError bool
	}{
		"empty": {},
		"plain": {
			note: "application server",
		},
		"leading whitespace": {
			note:      " application server",
			wantError: true,
		},
		"trailing whitespace": {
			note:      "application server ",
			wantError: true,
		},
		"control character": {
			note:      "application\nserver",
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateMySQLRemoteHostNote(test.note)
			if test.wantError && err == nil {
				t.Fatal("validateMySQLRemoteHostNote() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("validateMySQLRemoteHostNote() error: %v", err)
			}
		})
	}
}
