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
	Email                string          `json:"email"`
	User                 string          `json:"user"`
	Domain               string          `json:"domain"`
	DiskQuotaRaw         json.RawMessage `json:"_diskquota"`
	DiskUsedRaw          json.RawMessage `json:"_diskused"`
	HasSuspendedRaw      json.RawMessage `json:"has_suspended"`
	HoldOutgoingRaw      json.RawMessage `json:"hold_outgoing"`
	SuspendedIncomingRaw json.RawMessage `json:"suspended_incoming"`
	SuspendedLoginRaw    json.RawMessage `json:"suspended_login"`
	SuspendedOutgoingRaw json.RawMessage `json:"suspended_outgoing"`
}

type AccountSuspensions struct {
	Login        bool
	Incoming     bool
	Outgoing     bool
	OutgoingHeld bool
	HasSuspended bool
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

type DomainForwarderListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []DomainForwarder `json:"data"`
}

type DomainForwarder struct {
	Domain      string `json:"dest"`
	Destination string `json:"forward"`
}

type AutoResponderListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []AutoResponderSummary `json:"data"`
}

type AutoResponderSummary struct {
	Email   string `json:"email"`
	Subject string `json:"subject"`
}

type AutoResponderResponse struct {
	cpanel.UAPIDataSourceModel
	Data AutoResponder `json:"data"`
}

type AutoResponder struct {
	Email    string
	From     string `json:"from"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Charset  string `json:"charset"`
	Interval int64  `json:"interval"`
	IsHTML   int64  `json:"is_html"`
	Start    *int64 `json:"start"`
	Stop     *int64 `json:"stop"`
}

func (a AutoResponder) StartUnix() int64 {
	if a.Start == nil {
		return 0
	}

	return *a.Start
}

func (a AutoResponder) StopUnix() int64 {
	if a.Stop == nil {
		return 0
	}

	return *a.Stop
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

func (a Account) Suspensions() (AccountSuspensions, error) {
	login, err := parseBooleanFlagJSON(a.SuspendedLoginRaw)
	if err != nil {
		return AccountSuspensions{}, fmt.Errorf(
			"parse suspended_login for %q: %w",
			a.Email,
			err,
		)
	}
	incoming, err := parseBooleanFlagJSON(a.SuspendedIncomingRaw)
	if err != nil {
		return AccountSuspensions{}, fmt.Errorf(
			"parse suspended_incoming for %q: %w",
			a.Email,
			err,
		)
	}
	outgoing, err := parseBooleanFlagJSON(a.SuspendedOutgoingRaw)
	if err != nil {
		return AccountSuspensions{}, fmt.Errorf(
			"parse suspended_outgoing for %q: %w",
			a.Email,
			err,
		)
	}
	outgoingHeld, err := parseBooleanFlagJSON(a.HoldOutgoingRaw)
	if err != nil {
		return AccountSuspensions{}, fmt.Errorf(
			"parse hold_outgoing for %q: %w",
			a.Email,
			err,
		)
	}
	hasSuspended, err := parseBooleanFlagJSON(a.HasSuspendedRaw)
	if err != nil {
		return AccountSuspensions{}, fmt.Errorf(
			"parse has_suspended for %q: %w",
			a.Email,
			err,
		)
	}

	expectedHasSuspended := login || incoming || outgoing || outgoingHeld
	if hasSuspended != expectedHasSuspended {
		return AccountSuspensions{}, fmt.Errorf(
			"email account %q returned inconsistent suspension status",
			a.Email,
		)
	}

	return AccountSuspensions{
		Login:        login,
		Incoming:     incoming,
		Outgoing:     outgoing,
		OutgoingHeld: outgoingHeld,
		HasSuspended: hasSuspended,
	}, nil
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

func parseBooleanFlagJSON(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return false, fmt.Errorf("value is missing")
	}
	if bytes.Equal(value, []byte("null")) {
		return false, nil
	}

	switch string(value) {
	case "0", `"0"`, "false":
		return false, nil
	case "1", `"1"`, "true":
		return true, nil
	default:
		return false, fmt.Errorf("expected 0 or 1, got %q", value)
	}
}
