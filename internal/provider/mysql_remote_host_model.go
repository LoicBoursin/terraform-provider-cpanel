package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

type MySQLRemoteHostModel struct {
	Host types.String `tfsdk:"host"`
	Note types.String `tfsdk:"note"`
}

func applyMySQLRemoteHostToModel(
	model *MySQLRemoteHostModel,
	remoteHost mysql.RemoteHost,
) {
	model.Host = types.StringValue(remoteHost.Host)
	model.Note = types.StringValue(remoteHost.Note)
}
