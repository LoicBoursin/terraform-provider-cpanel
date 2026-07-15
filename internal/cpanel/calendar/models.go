package calendar

import (
	"encoding/json"

	"terraform-provider-cpanel/internal/cpanel"
)

const DefaultCalendar = "calendar"

type Delegate struct {
	Delegator    string
	Delegatee    string
	Calendar     string
	CalendarName string
	ReadOnly     bool
}

type Definition struct {
	Delegator string
	Delegatee string
	Calendar  string
	ReadOnly  bool
}

type User struct {
	Username    string
	Collections []Collection
}

type Collection struct {
	Name        string
	DisplayName string
	Type        string
}

type listDelegatesResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type listUsersResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiDelegate struct {
	Delegator      string          `json:"delegator"`
	Delegatee      string          `json:"delegatee"`
	Calendar       string          `json:"calendar"`
	CalendarName   string          `json:"calname"`
	ReadOnly       json.RawMessage `json:"readonly"`
	LegacyReadOnly json.RawMessage `json:"read_only"`
}

type apiCollection struct {
	DisplayName string `json:"displayname"`
	Type        string `json:"type"`
}
