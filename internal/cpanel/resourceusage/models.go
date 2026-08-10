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
	Formatter json.RawMessage `json:"formatter"`
	ID        string          `json:"id"`
	Maximum   json.RawMessage `json:"maximum"`
	Usage     json.RawMessage `json:"usage"`
}

type Metric struct {
	Formatter *string
	ID        string
	Maximum   *string
	Usage     string
}
