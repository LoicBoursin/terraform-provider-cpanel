package email

const (
	operationAddAccount       = "add_pop"
	operationAddAutoResponder = "add_auto_responder"
	operationAddDomainForward = "add_domain_forwarder"
	operationAddForwarder     = "add_forwarder"
	operationDeleteAccount    = "delete_pop"
	operationDeleteAutoResp   = "delete_auto_responder"
	operationDeleteDomainFwd  = "delete_domain_forwarder"
	operationDeleteForwarder  = "delete_forwarder"
	operationEditAccountQuota = "edit_pop_quota"
	operationGetAutoResponder = "get_auto_responder"
	operationListAccounts     = "list_pops_with_disk"
	operationListAutoResp     = "list_auto_responders"
	operationListDomainFwds   = "list_domain_forwarders"
	operationListForwarders   = "list_forwarders"
	operationListMailDomains  = "list_mail_domains"
	operationSetPassword      = "passwd_pop"
)
