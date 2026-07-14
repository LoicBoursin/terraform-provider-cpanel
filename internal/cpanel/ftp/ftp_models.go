package ftp

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type AccountListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Account `json:"data"`
}

type Account struct {
	Login             string          `json:"login"`
	User              string          `json:"user"`
	RelativeDirectory string          `json:"reldir"`
	AccountType       string          `json:"accttype"`
	DiskQuotaRaw      json.RawMessage `json:"_diskquota"`
	DiskUsedRaw       json.RawMessage `json:"_diskused"`
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

func (a Account) QuotaMiB() (int64, error) {
	value, err := parseDecimalJSON(a.DiskQuotaRaw)
	if err != nil {
		return 0, fmt.Errorf("parse FTP quota: %w", err)
	}
	if value < 0 || math.Trunc(value) != value {
		return 0, fmt.Errorf("FTP quota %v MiB is not a non-negative whole MiB value", value)
	}

	return int64(value), nil
}

func (a Account) DiskUsedMiB() (float64, error) {
	value, err := parseDecimalJSON(a.DiskUsedRaw)
	if err != nil {
		return 0, fmt.Errorf("parse FTP disk usage: %w", err)
	}

	return value, nil
}

func parseDecimalJSON(raw json.RawMessage) (float64, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" || value == `""` {
		return 0, nil
	}
	value = strings.Trim(value, `"`)
	if strings.EqualFold(value, "unlimited") {
		return 0, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as decimal: %w", value, err)
	}

	return parsed, nil
}
