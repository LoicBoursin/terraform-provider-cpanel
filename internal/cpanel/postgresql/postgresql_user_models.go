package postgresql

import "terraform-provider-cpanel/internal/cpanel"

type UserDataSourceModel struct {
	cpanel.UAPIDataSourceModel
	Data []string `tfsdk:"data"`
}

type UserCreateModel struct {
	Name     string `tfsdk:"name"`
	Password string `tfsdk:"password"`
}

type UserDeleteModel struct {
	Name string `tfsdk:"name"`
}

type UserGrantAllPrivilegesModel struct {
	Database string `tfsdk:"database"`
	User     string `tfsdk:"user"`
}

type UserRenameModel struct {
	NewName  string `tfsdk:"new_name"`
	OldName  string `tfsdk:"old_name"`
	Password string `tfsdk:"password"`
}

type UserRevokeAllPrivilegesModel struct {
	Database string `tfsdk:"database"`
	User     string `tfsdk:"user"`
}

type UserSetPasswordModel struct {
	Password string `tfsdk:"password"`
	User     string `tfsdk:"user"`
}
