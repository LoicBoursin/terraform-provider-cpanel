package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type FTPAccountResourceModel struct {
	Username            types.String `tfsdk:"username"`
	Password            types.String `tfsdk:"password"`
	PasswordVersion     types.Int64  `tfsdk:"password_version"`
	HomeDirectory       types.String `tfsdk:"home_directory"`
	QuotaMiB            types.Int64  `tfsdk:"quota_mib"`
	DeleteOnDestroy     types.Bool   `tfsdk:"delete_on_destroy"`
	DeleteHomeDirectory types.Bool   `tfsdk:"delete_home_directory"`
}

type FTPAccountDataSourceModel struct {
	Username      types.String  `tfsdk:"username"`
	HomeDirectory types.String  `tfsdk:"home_directory"`
	QuotaMiB      types.Int64   `tfsdk:"quota_mib"`
	DiskUsedMiB   types.Float64 `tfsdk:"disk_used_mib"`
}
