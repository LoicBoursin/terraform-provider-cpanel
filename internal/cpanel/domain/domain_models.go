package domain

import (
	"bytes"
	"encoding/json"
	"fmt"

	"terraform-provider-cpanel/internal/cpanel"
)

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

func (d *DomainList) UnmarshalJSON(data []byte) error {
	type rawDomainList struct {
		MainDomain    *string         `json:"main_domain"`
		AddonDomains  json.RawMessage `json:"addon_domains"`
		SubDomains    json.RawMessage `json:"sub_domains"`
		ParkedDomains json.RawMessage `json:"parked_domains"`
	}

	raw := rawDomainList{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.MainDomain == nil {
		return fmt.Errorf("main_domain must be a non-null string")
	}
	addonDomains, err := decodeRequiredDomainArray(
		raw.AddonDomains,
		"addon_domains",
	)
	if err != nil {
		return err
	}
	subDomains, err := decodeRequiredDomainArray(
		raw.SubDomains,
		"sub_domains",
	)
	if err != nil {
		return err
	}
	parkedDomains, err := decodeRequiredDomainArray(
		raw.ParkedDomains,
		"parked_domains",
	)
	if err != nil {
		return err
	}

	*d = DomainList{
		MainDomain:    *raw.MainDomain,
		AddonDomains:  addonDomains,
		SubDomains:    subDomains,
		ParkedDomains: parkedDomains,
	}

	return nil
}

func decodeRequiredDomainArray(
	raw json.RawMessage,
	field string,
) ([]string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s must be a non-null array", field)
	}

	result := make([]string, 0)
	if err := json.Unmarshal(value, &result); err != nil {
		return nil, fmt.Errorf("%s must be an array of strings: %w", field, err)
	}

	return result, nil
}

type InventoryDomainType string

const (
	InventoryDomainTypeMain      InventoryDomainType = "main"
	InventoryDomainTypeAddon     InventoryDomainType = "addon"
	InventoryDomainTypeSubdomain InventoryDomainType = "subdomain"
	InventoryDomainTypeAlias     InventoryDomainType = "alias"
)

type InventoryDomain struct {
	Name string
	Type InventoryDomainType
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

type DomainAliasListResponse struct {
	CpanelResult DomainAliasListResult `json:"cpanelresult"`
}

type DomainAliasListResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []DomainAlias `json:"data"`
}

type DomainAlias struct {
	Domain        string `json:"domain"`
	Directory     string `json:"dir"`
	RelativeDir   string `json:"reldir"`
	BaseDirectory string `json:"basedir"`
	Status        string `json:"status"`
}
