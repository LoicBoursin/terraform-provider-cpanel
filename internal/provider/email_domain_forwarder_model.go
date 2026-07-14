package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type EmailDomainForwarderModel struct {
	Domain      types.String `tfsdk:"domain"`
	Destination types.String `tfsdk:"destination"`
}
