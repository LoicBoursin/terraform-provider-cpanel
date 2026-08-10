package provider

import (
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	"terraform-provider-cpanel/internal/cpanel/passenger"
)

var passengerEnvironmentVariableNamePattern = regexp.MustCompile(
	`^[A-Za-z_-][A-Za-z0-9_-]*$`,
)

var passengerEnvironmentVariableValuePattern = regexp.MustCompile(
	`^[\x20-\x7e]+$`,
)

func passengerApplicationNameValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 50),
	}
}

func passengerApplicationPathValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func passengerApplicationBaseURIValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 2048),
	}
}

func passengerDeploymentModeValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			passenger.DeploymentModeDevelopment,
			passenger.DeploymentModeProduction,
		),
	}
}

func passengerEnvironmentVariableValidators() []validator.Map {
	return []validator.Map{
		mapvalidator.NoNullValues(),
		mapvalidator.KeysAre(
			stringvalidator.LengthBetween(1, 256),
			stringvalidator.RegexMatches(
				passengerEnvironmentVariableNamePattern,
				"must contain only letters, numbers, underscores, or dashes and must not begin with a number",
			),
		),
		mapvalidator.ValueStringsAre(
			stringvalidator.LengthBetween(1, 1024),
			stringvalidator.RegexMatches(
				passengerEnvironmentVariableValuePattern,
				"must contain only printable ASCII characters",
			),
		),
	}
}

func validatePassengerApplicationDefinition(
	definition passenger.Definition,
) error {
	if err := passenger.ValidateDefinition(definition); err != nil {
		return err
	}

	return validateDomainName(definition.Domain)
}
