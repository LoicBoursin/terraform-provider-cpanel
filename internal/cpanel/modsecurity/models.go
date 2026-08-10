package modsecurity

import (
	"encoding/json"
	"sort"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	DomainTypeMain = "main"
	DomainTypeSub  = "sub"
)

type Domain struct {
	Domain       string
	Enabled      bool
	Dependencies []string
	SearchHint   string
	Type         string
}

func (d Domain) AffectedDomains() []string {
	affected := make(map[string]struct{}, len(d.Dependencies)+1)
	affected[d.Domain] = struct{}{}
	for _, dependency := range d.Dependencies {
		affected[dependency] = struct{}{}
	}

	domains := make([]string, 0, len(affected))
	for domain := range affected {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	return domains
}

type Definition struct {
	Domain  string
	Enabled bool
}

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []apiDomain `json:"data"`
}

type InstalledResponse struct {
	cpanel.UAPIDataSourceModel
	Data installedData `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type installedData struct {
	Installed int `json:"installed"`
}

type apiDomain struct {
	Dependencies []string `json:"dependencies"`
	Domain       string   `json:"domain"`
	Enabled      int      `json:"enabled"`
	Exception    string   `json:"exception"`
	SearchHint   string   `json:"searchhint"`
	Type         string   `json:"type"`
}
