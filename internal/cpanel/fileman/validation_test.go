package fileman

import (
	"testing"
	"time"
)

func TestValidateManagedPath(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value     string
		wantError bool
	}{
		"file": {
			value: "public_html/index.html",
		},
		"nested directory": {
			value: "public_html/assets/images",
		},
		"nested cgi-bin name": {
			value: "public_html/assets/cgi-bin",
		},
		"unicode": {
			value: "public_html/français/index.html",
		},
		"empty": {
			value:     "",
			wantError: true,
		},
		"managed root": {
			value:     "public_html",
			wantError: true,
		},
		"outside managed root": {
			value:     "mail/index.html",
			wantError: true,
		},
		"similar prefix": {
			value:     "public_html_backup/index.html",
			wantError: true,
		},
		"absolute": {
			value:     "/home/example/public_html/index.html",
			wantError: true,
		},
		"parent segment": {
			value:     "public_html/../mail/index.html",
			wantError: true,
		},
		"current segment": {
			value:     "public_html/./index.html",
			wantError: true,
		},
		"double separator": {
			value:     "public_html/assets//index.html",
			wantError: true,
		},
		"trailing separator": {
			value:     "public_html/assets/",
			wantError: true,
		},
		"comma separator": {
			value:     "public_html/assets,index.html",
			wantError: true,
		},
		"cPanel cgi-bin": {
			value:     "public_html/cgi-bin",
			wantError: true,
		},
		"below cPanel cgi-bin": {
			value:     "public_html/cgi-bin/script.cgi",
			wantError: true,
		},
		"backslash": {
			value:     `public_html\index.html`,
			wantError: true,
		},
		"null byte": {
			value:     "public_html/index\x00.html",
			wantError: true,
		},
		"newline": {
			value:     "public_html/index\n.html",
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := ValidateManagedPath(testCase.value)
			if testCase.wantError && err == nil {
				t.Fatalf("ValidateManagedPath(%q) returned no error", testCase.value)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf(
					"ValidateManagedPath(%q) error: %v",
					testCase.value,
					err,
				)
			}
		})
	}
}

func TestLockMutationsIsGlobalAndUnlockIsIdempotent(t *testing.T) {
	unlockFirst := LockMutations()

	secondLock := make(chan func(), 1)
	go func() {
		secondLock <- (&Client{}).LockMutations()
	}()

	select {
	case unlock := <-secondLock:
		unlock()
		t.Fatal("second mutation lock did not wait")
	case <-time.After(50 * time.Millisecond):
	}

	unlockFirst()
	unlockFirst()

	select {
	case unlock := <-secondLock:
		unlock()
	case <-time.After(time.Second):
		t.Fatal("second mutation lock stayed blocked")
	}
}
