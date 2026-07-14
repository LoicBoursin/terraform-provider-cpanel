package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type DomainAliasModel struct {
	Domain       types.String `tfsdk:"domain"`
	TargetDomain types.String `tfsdk:"target_domain"`
	DocumentRoot types.String `tfsdk:"document_root"`
}
