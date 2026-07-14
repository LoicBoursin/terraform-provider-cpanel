package provider

import "terraform-provider-cpanel/internal/cpanel/modsecurity"

func validateModSecurityDomainDefinition(
	definition modsecurity.Definition,
) error {
	return validateDomainName(definition.Domain)
}
