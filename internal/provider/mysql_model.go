package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

type MySQLUserModel struct {
	Name            types.String `tfsdk:"name"`
	Password        types.String `tfsdk:"password"`
	PasswordVersion types.Int64  `tfsdk:"password_version"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

type MySQLUserDataSourceModel struct {
	Name types.String `tfsdk:"name"`
}

type MySQLDatabaseModel struct {
	Name            types.String `tfsdk:"name"`
	Users           types.Set    `tfsdk:"users"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

type MySQLDatabaseDataSourceModel struct {
	Name  types.String `tfsdk:"name"`
	Users types.Set    `tfsdk:"users"`
}

func MySQLDatabaseAPIToModel(
	ctx context.Context,
	response *mysql.DatabaseListResponse,
	name string,
) (*MySQLDatabaseDataSourceModel, diag.Diagnostics) {
	for _, database := range response.Data {
		if database.Database != name {
			continue
		}

		databaseUsers := database.Users
		if databaseUsers == nil {
			databaseUsers = []string{}
		}
		users, diagnostics := types.SetValueFrom(ctx, types.StringType, databaseUsers)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		return &MySQLDatabaseDataSourceModel{
			Name:  types.StringValue(database.Database),
			Users: users,
		}, diagnostics
	}

	return nil, nil
}
