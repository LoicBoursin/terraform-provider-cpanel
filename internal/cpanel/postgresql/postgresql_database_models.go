package postgresql

import "terraform-provider-cpanel/internal/cpanel"

type DatabaseDataSourceModel struct {
	cpanel.UAPIDataSourceModel
	Data []DatabaseDataSourceDataModel `json:"data" tfsdk:"data"`
}

type DatabaseDataSourceDataModel struct {
	Database  string   `json:"database" tfsdk:"database"`
	DiskUsage int64    `json:"disk_usage" tfsdk:"disk_usage"`
	Users     []string `json:"users" tfsdk:"users"`
}

type DatabaseCreateModel struct {
	Name string `json:"name" tfsdk:"name"`
}

type DatabaseUpdateModel struct {
	NewName string `json:"new_name" tfsdk:"new_name"`
	OldName string `json:"old_name" tfsdk:"old_name"`
}

type DatabaseDeleteModel struct {
	Name string `json:"name" tfsdk:"name"`
}
