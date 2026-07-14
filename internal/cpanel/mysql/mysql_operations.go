package mysql

const (
	operationCreateDatabase          = "create_database"
	operationCreateUser              = "create_user"
	operationDeleteDatabase          = "delete_database"
	operationDeleteRemoteHost        = "delete_host"
	operationDeleteUser              = "delete_user"
	operationGetPrivilegesOnDatabase = "get_privileges_on_database"
	operationGetRemoteHostNotes      = "get_host_notes"
	operationGetRestrictions         = "get_restrictions"
	operationAddRemoteHost           = "add_host"
	operationAddRemoteHostNote       = "add_host_note"
	operationListDatabases           = "list_databases"
	operationListRemoteHosts         = "listhosts"
	operationListUsers               = "list_users"
	operationRenameDatabase          = "rename_database"
	operationRenameUser              = "rename_user"
	operationRevokeAccessToDatabase  = "revoke_access_to_database"
	operationSetPassword             = "set_password"
	operationSetPrivilegesOnDatabase = "set_privileges_on_database"
	allPrivileges                    = "ALL PRIVILEGES"
)
