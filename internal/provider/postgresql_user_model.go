package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

type PostgreSQLUserModel struct {
	Name            types.String `tfsdk:"name"`
	Password        types.String `tfsdk:"password"`
	PasswordVersion types.Int64  `tfsdk:"password_version"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

type postgreSQLUserModelV0 struct {
	Name        types.String `tfsdk:"name"`
	Password    types.String `tfsdk:"password"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

type PostgreSQLUserDataSourceModel struct {
	Name types.String `tfsdk:"name"`
}

func PostgreSQLUserAPIToDataSourceModel(
	userDataSourceModel *postgresql.UserDataSourceModel,
	name string,
) *PostgreSQLUserDataSourceModel {
	for _, data := range userDataSourceModel.Data {
		if data != name {
			continue
		}

		return &PostgreSQLUserDataSourceModel{
			Name: types.StringValue(data),
		}
	}

	return nil
}
