package spamassassin

import (
	"encoding/json"
	"slices"

	"terraform-provider-cpanel/internal/cpanel"
)

const (
	PreferenceRequiredScore = "required_score"
	PreferenceScore         = "score"
	PreferenceWhitelistFrom = "whitelist_from"
	PreferenceBlacklistFrom = "blacklist_from"
)

type Preference struct {
	Name    string
	Values  []string
	Present bool
}

func (p Preference) Definition() Definition {
	return Definition{
		Name:    p.Name,
		Values:  slices.Clone(p.Values),
		Present: p.Present,
	}.Sorted()
}

type Definition struct {
	Name    string
	Values  []string
	Present bool
}

func (d Definition) Sorted() Definition {
	result := Definition{
		Name:    d.Name,
		Values:  slices.Clone(d.Values),
		Present: d.Present,
	}
	slices.Sort(result.Values)

	return result
}

type preferencesResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}
