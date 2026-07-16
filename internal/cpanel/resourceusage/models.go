package resourceusage

import (
	"encoding/json"

	"terraform-provider-cpanel/internal/cpanel"
)

const AccountIdentity = "account"

type response struct {
	cpanel.UAPIDataSourceModel
	Data []apiMetric `json:"data"`
}

type apiMetric struct {
	Description string          `json:"description"`
	Error       json.RawMessage `json:"error"`
	Formatter   json.RawMessage `json:"formatter"`
	ID          string          `json:"id"`
	Maximum     json.RawMessage `json:"maximum"`
	Usage       json.RawMessage `json:"usage"`
}

type Metric struct {
	Description string
	Error       *string
	Formatter   *string
	ID          string
	Maximum     *string
	Usage       string
}
