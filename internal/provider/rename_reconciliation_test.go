package provider

import "testing"

func TestReconcileRenamePresence(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		oldExists   bool
		newExists   bool
		wantApplied bool
		wantError   bool
	}{
		"new identity cannot be attributed": {
			newExists: true,
			wantError: true,
		},
		"not applied": {
			oldExists: true,
		},
		"both exist": {
			oldExists: true,
			newExists: true,
			wantError: true,
		},
		"neither exists": {
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			applied, err := reconcileRenamePresence(
				test.oldExists,
				test.newExists,
			)
			if test.wantError && err == nil {
				t.Fatal("reconcileRenamePresence() error = nil, want error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("reconcileRenamePresence() error = %v", err)
			}
			if applied != test.wantApplied {
				t.Fatalf(
					"reconcileRenamePresence() applied = %t, want %t",
					applied,
					test.wantApplied,
				)
			}
		})
	}
}
