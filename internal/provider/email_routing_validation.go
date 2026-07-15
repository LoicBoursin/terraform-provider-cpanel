package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func emailRoutingModeValidators() []validator.String {
	return []validator.String{
		stringvalidator.OneOf(
			string(cpanelmail.RoutingModeAuto),
			string(cpanelmail.RoutingModeBackup),
			string(cpanelmail.RoutingModeLocal),
			string(cpanelmail.RoutingModeRemote),
		),
	}
}

func validateEmailRoutingDefinition(
	definition cpanelmail.RoutingDefinition,
) error {
	if err := validateDomainName(definition.Domain); err != nil {
		return fmt.Errorf("invalid email routing domain: %w", err)
	}
	if err := cpanelmail.ValidateRoutingDefinition(definition); err != nil {
		return err
	}

	return nil
}
