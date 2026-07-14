package email

const (
	operationAddAccount       = "add_pop"
	operationAddDomainForward = "add_domain_forwarder"
	operationAddForwarder     = "add_forwarder"
	operationDeleteAccount    = "delete_pop"
	operationDeleteDomainFwd  = "delete_domain_forwarder"
	operationDeleteForwarder  = "delete_forwarder"
	operationEditAccountQuota = "edit_pop_quota"
	operationListAccounts     = "list_pops_with_disk"
	operationListDomainFwds   = "list_domain_forwarders"
	operationListForwarders   = "list_forwarders"
	operationListMailDomains  = "list_mail_domains"
	operationSetPassword      = "passwd_pop"
)
