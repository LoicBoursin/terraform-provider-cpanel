package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type EmailForwarderModel struct {
	Address     types.String `tfsdk:"address"`
	Destination types.String `tfsdk:"destination"`
}
