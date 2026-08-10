package postgresql

import "terraform-provider-cpanel/internal/cpanel"

type UserDataSourceModel struct {
	cpanel.UAPIDataSourceModel
	Data []string `json:"data" tfsdk:"data"`
}

type UserCreateModel struct {
	Name     string `json:"name" tfsdk:"name"`
	Password string `json:"password" tfsdk:"password"`
}

type UserDeleteModel struct {
	Name string `json:"name" tfsdk:"name"`
}

type UserGrantAllPrivilegesModel struct {
	Database string `json:"database" tfsdk:"database"`
	User     string `json:"user" tfsdk:"user"`
}

type UserRenameModel struct {
	NewName  string `json:"new_name" tfsdk:"new_name"`
	OldName  string `json:"old_name" tfsdk:"old_name"`
	Password string `json:"password" tfsdk:"password"`
}

type UserRevokeAllPrivilegesModel struct {
	Database string `json:"database" tfsdk:"database"`
	User     string `json:"user" tfsdk:"user"`
}

type UserSetPasswordModel struct {
	Password string `json:"password" tfsdk:"password"`
	User     string `json:"user" tfsdk:"user"`
}
