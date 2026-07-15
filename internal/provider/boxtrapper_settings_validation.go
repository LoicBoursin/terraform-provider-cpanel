package provider

import (
	"fmt"
	"strings"
	"unicode"
)

func validateBoxTrapperAccount(account string) error {
	if account == "" {
		return fmt.Errorf("BoxTrapper account must not be empty")
	}
	if strings.TrimSpace(account) != account {
		return fmt.Errorf(
			"BoxTrapper account %q must not contain surrounding whitespace",
			account,
		)
	}
	if len(account) > 254 {
		return fmt.Errorf(
			"BoxTrapper account must not exceed 254 ASCII characters",
		)
	}
	for _, character := range account {
		if character > unicode.MaxASCII {
			return fmt.Errorf(
				"BoxTrapper account must contain only ASCII characters",
			)
		}
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return fmt.Errorf(
				"BoxTrapper account must not contain whitespace or control characters",
			)
		}
	}

	if strings.Contains(account, "@") {
		if _, _, err := splitEmailAccountAddress(account); err != nil {
			return fmt.Errorf("invalid BoxTrapper email account: %w", err)
		}

		return nil
	}
	if len(account) > 64 {
		return fmt.Errorf(
			"BoxTrapper system account must not exceed 64 ASCII characters",
		)
	}
	for index, character := range account {
		isLetter := character >= 'A' && character <= 'Z' ||
			character >= 'a' && character <= 'z'
		isNumber := character >= '0' && character <= '9'
		if !isLetter && !isNumber && character != '_' && character != '-' {
			return fmt.Errorf(
				"BoxTrapper system account may contain only letters, numbers, underscores, and hyphens",
			)
		}
		if index == 0 && !isLetter && !isNumber {
			return fmt.Errorf(
				"BoxTrapper system account must start with a letter or number",
			)
		}
	}

	return nil
}
