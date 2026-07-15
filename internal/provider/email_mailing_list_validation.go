package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func emailMailingListPasswordValidators() []validator.String {
	return []validator.String{
		stringvalidator.LengthBetween(1, 4096),
	}
}

func validateEmailMailingListAddress(
	address string,
) (string, error) {
	_, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		return "", fmt.Errorf("invalid mailing list address: %w", err)
	}

	return domain, nil
}
