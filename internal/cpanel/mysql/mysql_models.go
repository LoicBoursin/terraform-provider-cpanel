package mysql

import "terraform-provider-cpanel/internal/cpanel"

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
}

type DatabaseListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Database `json:"data"`
}

type Database struct {
	Database  string   `json:"database"`
	DiskUsage int64    `json:"disk_usage"`
	Users     []string `json:"users"`
}

type UserListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []User `json:"data"`
}

type User struct {
	User      string   `json:"user"`
	ShortUser string   `json:"shortuser"`
	Databases []string `json:"databases"`
}

type RestrictionsResponse struct {
	cpanel.UAPIDataSourceModel
	Data Restrictions `json:"data"`
}

type Restrictions struct {
	MaxUsernameLength     int    `json:"max_username_length"`
	MaxDatabaseNameLength int    `json:"max_database_name_length"`
	Prefix                string `json:"prefix"`
}

type PrivilegesResponse struct {
	cpanel.UAPIDataSourceModel
	Data []string `json:"data"`
}

type SetPasswordResponse struct {
	cpanel.UAPIDataSourceModel
	Data SetPasswordData `json:"data"`
}

type SetPasswordData struct {
	Failures []SetPasswordFailure `json:"failures"`
}

type SetPasswordFailure struct {
	Error string `json:"error"`
	Host  string `json:"host"`
}
