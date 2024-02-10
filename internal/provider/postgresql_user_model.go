package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

type PostgreSQLUserModel struct {
	Name        types.String `tfsdk:"name"`
	Password    types.String `tfsdk:"password"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

func PostgreSQLUserAPIToModel(UserDataSourceModel *postgresql.UserDataSourceModel, name string) *PostgreSQLUserModel {
	for _, data := range UserDataSourceModel.Data {
		if data != name {
			continue
		}

		return &PostgreSQLUserModel{
			Name: types.StringValue(data),
		}
	}

	return nil
}
