package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type IPBlockModel struct {
	Address      types.String `tfsdk:"address"`
	StartAddress types.String `tfsdk:"start_address"`
	EndAddress   types.String `tfsdk:"end_address"`
}
