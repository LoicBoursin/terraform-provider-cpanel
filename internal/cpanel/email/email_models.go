package email

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

const bytesPerMiB = 1024 * 1024

type AccountListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Account `json:"data"`
}

type Account struct {
	Email        string          `json:"email"`
	User         string          `json:"user"`
	Domain       string          `json:"domain"`
	DiskQuotaRaw json.RawMessage `json:"_diskquota"`
	DiskUsedRaw  json.RawMessage `json:"_diskused"`
}

type MailDomainListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []MailDomain `json:"data"`
}

type MailDomain struct {
	Domain string `json:"domain"`
}

type ForwarderListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Forwarder `json:"data"`
}

type Forwarder struct {
	Address     string `json:"dest"`
	Destination string `json:"forward"`
}

func (a Account) QuotaMiB() (int64, error) {
	bytesValue, err := parseIntegerJSON(a.DiskQuotaRaw)
	if err != nil {
		return 0, fmt.Errorf("parse email quota: %w", err)
	}
	if bytesValue == 0 {
		return 0, nil
	}
	if bytesValue%bytesPerMiB != 0 {
		return 0, fmt.Errorf("email quota %d bytes is not a whole MiB value", bytesValue)
	}

	return bytesValue / bytesPerMiB, nil
}

func (a Account) DiskUsedBytes() (int64, error) {
	value, err := parseIntegerJSON(a.DiskUsedRaw)
	if err != nil {
		return 0, fmt.Errorf("parse email disk usage: %w", err)
	}

	return value, nil
}

func parseIntegerJSON(raw json.RawMessage) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, nil
	}
	value = bytes.Trim(value, `"`)

	parsed, err := strconv.ParseInt(string(value), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as integer: %w", value, err)
	}

	return parsed, nil
}
