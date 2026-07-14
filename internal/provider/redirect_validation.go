package provider

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

func redirectSourceValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 2048),
	}
}

func redirectDestinationValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func redirectTypeValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			cpanelredirect.TypePermanent,
			cpanelredirect.TypeTemporary,
		),
	}
}

func redirectWWWModeValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			cpanelredirect.WWWModeBoth,
			cpanelredirect.WWWModeWithout,
		),
	}
}

func validateRedirectSource(source string) error {
	if !strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "//") {
		return fmt.Errorf("redirect source must be an absolute URL path")
	}
	if containsURLWhitespaceOrControl(source) {
		return fmt.Errorf(
			"redirect source must percent-encode whitespace and control characters",
		)
	}
	if strings.ContainsAny(source, "?#|") {
		return fmt.Errorf(
			"redirect source must not contain a query, fragment, or pipe character",
		)
	}

	parsed, err := url.ParseRequestURI(source)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return fmt.Errorf("redirect source must be a valid URL path")
	}

	return nil
}

func validateRedirectDestination(destination string) error {
	if containsURLWhitespaceOrControl(destination) {
		return fmt.Errorf(
			"redirect destination must percent-encode whitespace and control characters",
		)
	}

	parsed, err := url.ParseRequestURI(destination)
	if err != nil {
		return fmt.Errorf("redirect destination must be a valid absolute URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("redirect destination must use HTTP or HTTPS")
	}
	if parsed.Host == "" {
		return fmt.Errorf("redirect destination must contain a host")
	}
	if parsed.User != nil {
		return fmt.Errorf("redirect destination must not contain credentials")
	}

	return nil
}

func containsURLWhitespaceOrControl(value string) bool {
	return strings.IndexFunc(value, func(character rune) bool {
		return unicode.IsSpace(character) || unicode.IsControl(character)
	}) >= 0
}
