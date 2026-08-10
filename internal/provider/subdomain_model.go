package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type SubdomainResourceModel struct {
	Domain             types.String `tfsdk:"domain"`
	Subdomain          types.String `tfsdk:"subdomain"`
	RootDomain         types.String `tfsdk:"root_domain"`
	DocumentRoot       types.String `tfsdk:"document_root"`
	DeleteDocumentRoot types.Bool   `tfsdk:"delete_document_root"`
}

type SubdomainDataSourceModel struct {
	Domain       types.String `tfsdk:"domain"`
	Subdomain    types.String `tfsdk:"subdomain"`
	RootDomain   types.String `tfsdk:"root_domain"`
	DocumentRoot types.String `tfsdk:"document_root"`
}
