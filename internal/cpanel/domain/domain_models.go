package domain

import "terraform-provider-cpanel/internal/cpanel"

type API2MutationResponse struct {
	CpanelResult API2MutationResult `json:"cpanelresult"`
}

type API2MutationResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []MutationResult `json:"data"`
}

type MutationResult struct {
	Result int    `json:"result"`
	Reason string `json:"reason"`
}

type DomainListResponse struct {
	cpanel.UAPIDataSourceModel
	Data DomainList `json:"data"`
}

type DomainList struct {
	MainDomain    string   `json:"main_domain"`
	AddonDomains  []string `json:"addon_domains"`
	SubDomains    []string `json:"sub_domains"`
	ParkedDomains []string `json:"parked_domains"`
}

type SubdomainListResponse struct {
	CpanelResult SubdomainListResult `json:"cpanelresult"`
}

type SubdomainListResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []Subdomain `json:"data"`
}

type Subdomain struct {
	Domain        string `json:"domain"`
	DomainKey     string `json:"domainkey"`
	Subdomain     string `json:"subdomain"`
	RootDomain    string `json:"rootdomain"`
	Directory     string `json:"dir"`
	RelativeDir   string `json:"reldir"`
	BaseDirectory string `json:"basedir"`
}

type AddonDomainListResponse struct {
	CpanelResult AddonDomainListResult `json:"cpanelresult"`
}

type AddonDomainListResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []AddonDomain `json:"data"`
}

type AddonDomain struct {
	Domain            string `json:"domain"`
	DomainKey         string `json:"domainkey"`
	InternalSubdomain string `json:"subdomain"`
	RootDomain        string `json:"rootdomain"`
	FullSubdomain     string `json:"fullsubdomain"`
	Directory         string `json:"dir"`
	RelativeDir       string `json:"reldir"`
	BaseDirectory     string `json:"basedir"`
	Status            string `json:"status"`
}
