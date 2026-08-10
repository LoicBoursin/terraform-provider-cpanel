package fileman

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

const managedRoot = "public_html"

func ValidateManagedPath(managedPath string) error {
	if managedPath == "" {
		return fmt.Errorf("managed path must not be empty")
	}
	if path.IsAbs(managedPath) {
		return fmt.Errorf("managed path must be relative to the cPanel account home")
	}
	if managedPath == managedRoot {
		return fmt.Errorf("managed path must be strictly below %s/", managedRoot)
	}
	if !strings.HasPrefix(managedPath, managedRoot+"/") {
		return fmt.Errorf("managed path must be strictly below %s/", managedRoot)
	}
	if path.Clean(managedPath) != managedPath {
		return fmt.Errorf(
			"managed path must be normalized without empty, . or .. segments",
		)
	}
	if strings.ContainsRune(managedPath, ',') {
		return fmt.Errorf(
			"managed path must not contain commas because cPanel treats them as path separators",
		)
	}
	if strings.ContainsRune(managedPath, '\\') {
		return fmt.Errorf("managed path must not contain backslashes")
	}
	if containsControlCharacter(managedPath) {
		return fmt.Errorf("managed path must not contain control characters")
	}
	relativePath := strings.TrimPrefix(managedPath, managedRoot+"/")
	if firstSegment := strings.SplitN(relativePath, "/", 2)[0]; firstSegment == "cgi-bin" {
		return fmt.Errorf(
			"managed path must not use cPanel-controlled directory %q",
			firstSegment,
		)
	}

	return nil
}

func validateResolvablePath(managedPath string, allowManagedRoot bool) error {
	if allowManagedRoot && managedPath == managedRoot {
		return nil
	}

	return ValidateManagedPath(managedPath)
}

func validateEntryName(name string) error {
	if name == "" {
		return fmt.Errorf("entry name must not be empty")
	}
	if strings.ContainsRune(name, '/') ||
		path.Clean(name) != name || path.Base(name) != name ||
		name == "." || name == ".." {
		return fmt.Errorf("entry name %q must be a normalized path segment", name)
	}
	if strings.ContainsRune(name, '\\') {
		return fmt.Errorf("entry name %q must not contain backslashes", name)
	}
	if containsControlCharacter(name) {
		return fmt.Errorf(
			"entry name %q must not contain control characters",
			name,
		)
	}

	return nil
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}

	return false
}
