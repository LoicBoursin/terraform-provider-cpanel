package apitoken

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Token `json:"data"`
}

type CreateResponse struct {
	cpanel.UAPIDataSourceModel
	Data CreatedToken `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type Token struct {
	Name          string                `json:"name"`
	HasFullAccess int                   `json:"has_full_access"`
	ExpiresAt     NullableUnixTimestamp `json:"expires_at"`
	CreateTime    int64                 `json:"create_time"`
	Features      []string              `json:"features"`
	WhitelistIPs  []string              `json:"whitelist_ips"`
}

type CreatedToken struct {
	Token        string   `json:"token"`
	CreateTime   int64    `json:"create_time"`
	WhitelistIPs []string `json:"whitelist_ips"`
}

type NullableUnixTimestamp struct {
	Value int64
	Valid bool
}

func (timestamp *NullableUnixTimestamp) UnmarshalJSON(value []byte) error {
	value = bytes.TrimSpace(value)
	if bytes.Equal(value, []byte("null")) || bytes.Equal(value, []byte(`""`)) {
		timestamp.Value = 0
		timestamp.Valid = false

		return nil
	}

	var text string
	if len(value) > 0 && value[0] == '"' {
		if err := json.Unmarshal(value, &text); err != nil {
			return fmt.Errorf("decode quoted Unix timestamp: %w", err)
		}
	} else {
		text = string(value)
	}

	parsed, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return fmt.Errorf("parse Unix timestamp %q: %w", text, err)
	}

	timestamp.Value = parsed
	timestamp.Valid = true

	return nil
}

func (timestamp NullableUnixTimestamp) ValueOrZero() int64 {
	if !timestamp.Valid {
		return 0
	}

	return timestamp.Value
}
