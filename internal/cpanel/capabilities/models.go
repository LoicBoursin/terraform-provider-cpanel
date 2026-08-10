package capabilities

import (
	"encoding/json"

	"terraform-provider-cpanel/internal/cpanel"
)

const AccountIdentity = "account"

type featureListResponse struct {
	cpanel.UAPIDataSourceModel
	Data map[string]json.RawMessage `json:"data"`
}

type statsResponse struct {
	cpanel.UAPIDataSourceModel
	Data []apiStat `json:"data"`
}

type userInformationResponse struct {
	cpanel.UAPIDataSourceModel
	Data apiUserInformation `json:"data"`
}

type apiStat struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type apiUserInformation struct {
	Home                         string `json:"home"`
	MaximumAddonDomains          string `json:"maximum_addon_domains"`
	MaximumDatabases             string `json:"maximum_databases"`
	MaximumDeferFailPercentage   string `json:"maximum_defer_fail_percentage"`
	MaximumEmailAccountDiskQuota string `json:"maximum_email_account_disk_quota"`
	MaximumEmailsPerHour         string `json:"maximum_emails_per_hour"`
	MaximumFTPAccounts           string `json:"maximum_ftp_accounts"`
	MaximumMailAccounts          string `json:"maximum_mail_accounts"`
	MaximumMailingLists          string `json:"maximum_mailing_lists"`
	MaximumParkedDomains         string `json:"maximum_parked_domains"`
	MaximumPassengerApps         string `json:"maximum_passenger_apps"`
	MaximumSubdomains            string `json:"maximum_subdomains"`
	MaximumTeamUsers             string `json:"max_team_users"`
	Plan                         string `json:"plan"`
	PrimaryDomain                string `json:"domain"`
	Shell                        string `json:"shell"`
	Theme                        string `json:"theme"`
	Username                     string `json:"user"`
}

type AccountCapabilities struct {
	Version       string
	Username      string
	HomeDirectory string
	PrimaryDomain string
	Plan          string
	Theme         string
	Shell         string
	Features      map[string]bool
	Limits        map[string]string
}
