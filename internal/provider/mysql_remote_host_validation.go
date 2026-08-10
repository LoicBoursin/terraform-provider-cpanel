package provider

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func mySQLRemoteHostValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 253),
	}
}

func mySQLRemoteHostNoteValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthAtMost(4096),
	}
}

func validateMySQLRemoteHostNote(note string) error {
	if strings.TrimSpace(note) != note {
		return fmt.Errorf(
			"remote MySQL host note must not have leading or trailing whitespace",
		)
	}
	for _, character := range note {
		if unicode.IsControl(character) {
			return fmt.Errorf(
				"remote MySQL host note must not contain control characters",
			)
		}
	}

	return nil
}
