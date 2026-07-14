package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ipblock"
)

func applyIPBlockToModel(
	model *IPBlockModel,
	blockedAddress ipblock.BlockedAddress,
) error {
	address, err := ipblock.NormalizeAddress(blockedAddress.Address)
	if err != nil {
		return fmt.Errorf("normalize blocked address: %w", err)
	}
	start, err := ipblock.NormalizeAddress(blockedAddress.Start)
	if err != nil {
		return fmt.Errorf("normalize blocked range start: %w", err)
	}
	end, err := ipblock.NormalizeAddress(blockedAddress.End)
	if err != nil {
		return fmt.Errorf("normalize blocked range end: %w", err)
	}

	model.Address = types.StringValue(address)
	model.StartAddress = types.StringValue(start)
	model.EndAddress = types.StringValue(end)

	return nil
}
