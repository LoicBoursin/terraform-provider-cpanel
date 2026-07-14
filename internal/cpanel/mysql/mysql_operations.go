package mysql

const (
	operationCreateDatabase          = "create_database"
	operationCreateUser              = "create_user"
	operationDeleteDatabase          = "delete_database"
	operationDeleteUser              = "delete_user"
	operationGetPrivilegesOnDatabase = "get_privileges_on_database"
	operationGetRestrictions         = "get_restrictions"
	operationListDatabases           = "list_databases"
	operationListUsers               = "list_users"
	operationRenameDatabase          = "rename_database"
	operationRenameUser              = "rename_user"
	operationRevokeAccessToDatabase  = "revoke_access_to_database"
	operationSetPassword             = "set_password"
	operationSetPrivilegesOnDatabase = "set_privileges_on_database"
	allPrivileges                    = "ALL PRIVILEGES"
)
