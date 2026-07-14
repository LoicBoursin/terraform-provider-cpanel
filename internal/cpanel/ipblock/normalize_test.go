package ipblock

import "testing"

func TestNormalizeAddress(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value     string
		expected  string
		wantError bool
	}{
		"ipv4": {
			value:    "198.51.100.77",
			expected: "198.51.100.77",
		},
		"ipv6": {
			value:    "2001:0db8:0000:0000:0000:0000:0000:0123",
			expected: "2001:db8::123",
		},
		"cidr": {
			value:    "203.0.113.65/30",
			expected: "203.0.113.64/30",
		},
		"ipv6 cidr": {
			value:    "2001:db8:ffff::123/120",
			expected: "2001:db8:ffff::100/120",
		},
		"range": {
			value:    "198.51.100.80-198.51.100.90",
			expected: "198.51.100.80-198.51.100.90",
		},
		"reverse range": {
			value:     "198.51.100.90-198.51.100.80",
			wantError: true,
		},
		"mixed range": {
			value:     "198.51.100.80-2001:db8::1",
			wantError: true,
		},
		"invalid": {
			value:     "not-an-ip",
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := NormalizeAddress(testCase.value)
			if testCase.wantError {
				if err == nil {
					t.Fatalf("NormalizeAddress(%q) returned no error", testCase.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeAddress(%q) error: %v", testCase.value, err)
			}
			if got != testCase.expected {
				t.Fatalf(
					"NormalizeAddress(%q) = %q, want %q",
					testCase.value,
					got,
					testCase.expected,
				)
			}
		})
	}
}

func TestNormalizeAddressRange(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value         string
		expectedStart string
		expectedEnd   string
	}{
		"IPv4 address": {
			value:         "198.51.100.77",
			expectedStart: "198.51.100.77",
			expectedEnd:   "198.51.100.77",
		},
		"IPv4 CIDR": {
			value:         "203.0.113.65/30",
			expectedStart: "203.0.113.64",
			expectedEnd:   "203.0.113.67",
		},
		"IPv6 CIDR": {
			value:         "2001:db8:ffff::123/120",
			expectedStart: "2001:db8:ffff::100",
			expectedEnd:   "2001:db8:ffff::1ff",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, start, end, err := normalizeAddressRange(testCase.value)
			if err != nil {
				t.Fatalf("normalizeAddressRange(%q) error: %v", testCase.value, err)
			}
			if start.String() != testCase.expectedStart ||
				end.String() != testCase.expectedEnd {
				t.Fatalf(
					"normalizeAddressRange(%q) = %s-%s, want %s-%s",
					testCase.value,
					start,
					end,
					testCase.expectedStart,
					testCase.expectedEnd,
				)
			}
		})
	}
}
