package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"
	"terraform-provider-cpanel/internal/cpanel/postgresql"
	"time"
)

type PostgreSQLDatabaseModel struct {
	Name        types.String   `tfsdk:"name"`
	Users       []types.String `tfsdk:"users"`
	LastUpdated types.String   `tfsdk:"last_updated"`
}

func PostgreSQLDatabaseAPIToModel(databaseDataSourceModel *postgresql.DatabaseDataSourceModel, name string) *PostgreSQLDatabaseModel {
	for _, data := range databaseDataSourceModel.Data {
		if data.Database != name {
			continue
		}

		users := make([]types.String, 0, len(data.Users))
		for _, user := range data.Users {
			users = append(users, types.StringValue(user))
		}

		return &PostgreSQLDatabaseModel{
			Name:        types.StringValue(data.Database),
			Users:       users,
			LastUpdated: types.StringValue(time.Now().Format(time.RFC3339)),
		}
	}

	return nil
}
