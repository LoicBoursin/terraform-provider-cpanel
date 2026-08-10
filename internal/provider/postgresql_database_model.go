package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/postgresql"
)

type PostgreSQLDatabaseModel struct {
	Name            types.String `tfsdk:"name"`
	Users           types.Set    `tfsdk:"users"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
}

type postgreSQLDatabaseModelV0 struct {
	Name        types.String `tfsdk:"name"`
	Users       types.List   `tfsdk:"users"`
	LastUpdated types.String `tfsdk:"last_updated"`
}

type PostgreSQLDatabaseDataSourceModel struct {
	Name  types.String `tfsdk:"name"`
	Users types.Set    `tfsdk:"users"`
}

func PostgreSQLDatabaseAPIToModel(
	ctx context.Context,
	databaseDataSourceModel *postgresql.DatabaseDataSourceModel,
	name string,
) (*PostgreSQLDatabaseDataSourceModel, diag.Diagnostics) {
	for _, data := range databaseDataSourceModel.Data {
		if data.Database != name {
			continue
		}

		databaseUsers := data.Users
		if databaseUsers == nil {
			databaseUsers = []string{}
		}
		users, diagnostics := types.SetValueFrom(ctx, types.StringType, databaseUsers)
		if diagnostics.HasError() {
			return nil, diagnostics
		}

		return &PostgreSQLDatabaseDataSourceModel{
			Name:  types.StringValue(data.Database),
			Users: users,
		}, diagnostics
	}

	return nil, nil
}
