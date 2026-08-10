package apitoken

import (
	"encoding/json"
	"testing"
)

func TestNullableUnixTimestampUnmarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		value         string
		expectedValue int64
		expectedValid bool
		wantError     bool
	}{
		"null": {
			value: "null",
		},
		"empty string": {
			value: `""`,
		},
		"quoted": {
			value:         `"1784131088"`,
			expectedValue: 1784131088,
			expectedValid: true,
		},
		"number": {
			value:         "1784131088",
			expectedValue: 1784131088,
			expectedValid: true,
		},
		"invalid": {
			value:     `"tomorrow"`,
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var timestamp NullableUnixTimestamp
			err := json.Unmarshal([]byte(testCase.value), &timestamp)
			if testCase.wantError {
				if err == nil {
					t.Fatalf("Unmarshal(%s) returned no error", testCase.value)
				}

				return
			}
			if err != nil {
				t.Fatalf("Unmarshal(%s) error: %v", testCase.value, err)
			}
			if timestamp.Value != testCase.expectedValue ||
				timestamp.Valid != testCase.expectedValid {
				t.Fatalf(
					"timestamp = %#v, want value %d valid %t",
					timestamp,
					testCase.expectedValue,
					testCase.expectedValid,
				)
			}
		})
	}
}
