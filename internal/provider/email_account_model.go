package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type EmailAccountResourceModel struct {
	Email           types.String `tfsdk:"email"`
	Password        types.String `tfsdk:"password"`
	PasswordVersion types.Int64  `tfsdk:"password_version"`
	QuotaMiB        types.Int64  `tfsdk:"quota_mib"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

type EmailAccountDataSourceModel struct {
	Email         types.String `tfsdk:"email"`
	QuotaMiB      types.Int64  `tfsdk:"quota_mib"`
	DiskUsedBytes types.Int64  `tfsdk:"disk_used_bytes"`
}
