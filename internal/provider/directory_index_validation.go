package provider

import (
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func directoryIndexDirectoryValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func directoryIndexTypeValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			directoryindex.IndexTypeDisabled,
			directoryindex.IndexTypeFancy,
			directoryindex.IndexTypeInherit,
			directoryindex.IndexTypeStandard,
		),
	}
}

func validateDirectoryIndexDefinition(
	definition directoryindex.Definition,
) error {
	if definition.Directory == "" {
		return fmt.Errorf("directory must not be empty")
	}
	if len(definition.Directory) > 4096 {
		return fmt.Errorf("directory must not exceed 4096 bytes")
	}
	if path.IsAbs(definition.Directory) {
		return fmt.Errorf(
			"directory must be relative to the cPanel account home",
		)
	}
	if strings.ContainsRune(definition.Directory, '\x00') {
		return fmt.Errorf("directory must not contain a null byte")
	}
	if path.Clean(definition.Directory) != definition.Directory ||
		definition.Directory == "." ||
		definition.Directory == ".." ||
		strings.HasPrefix(definition.Directory, "../") {
		return fmt.Errorf(
			"directory must be a normalized relative path without . or .. segments",
		)
	}

	switch definition.Type {
	case directoryindex.IndexTypeDisabled,
		directoryindex.IndexTypeFancy,
		directoryindex.IndexTypeInherit,
		directoryindex.IndexTypeStandard:
		return nil
	default:
		return fmt.Errorf(
			"unsupported directory indexing type %q",
			definition.Type,
		)
	}
}
