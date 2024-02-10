package postgresql

import "terraform-provider-cpanel/internal/cpanel"

type DatabaseDataSourceModel struct {
	cpanel.UAPIDataSourceModel
	Data []DatabaseDataSourceDataModel `tfsdk:"data"`
}

type DatabaseDataSourceDataModel struct {
	Database  string   `tfsdk:"database"`
	DiskUsage int64    `tfsdk:"disk_usage"`
	Users     []string `tfsdk:"users"`
}

type DatabaseCreateModel struct {
	Name string `tfsdk:"name"`
}

type DatabaseUpdateModel struct {
	NewName string `tfsdk:"new_name"`
	OldName string `tfsdk:"old_name"`
}

type DatabaseDeleteModel struct {
	Name string `tfsdk:"name"`
}
