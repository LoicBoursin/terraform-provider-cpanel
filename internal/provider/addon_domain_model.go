package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type AddonDomainResourceModel struct {
	Domain             types.String `tfsdk:"domain"`
	InternalSubdomain  types.String `tfsdk:"internal_subdomain"`
	RootDomain         types.String `tfsdk:"root_domain"`
	FullSubdomain      types.String `tfsdk:"full_subdomain"`
	DomainKey          types.String `tfsdk:"domain_key"`
	DocumentRoot       types.String `tfsdk:"document_root"`
	DeleteDocumentRoot types.Bool   `tfsdk:"delete_document_root"`
}

type AddonDomainDataSourceModel struct {
	Domain            types.String `tfsdk:"domain"`
	InternalSubdomain types.String `tfsdk:"internal_subdomain"`
	RootDomain        types.String `tfsdk:"root_domain"`
	FullSubdomain     types.String `tfsdk:"full_subdomain"`
	DomainKey         types.String `tfsdk:"domain_key"`
	DocumentRoot      types.String `tfsdk:"document_root"`
}
