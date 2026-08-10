package cron

import (
	"encoding/json"
	"fmt"
	"strconv"

	"terraform-provider-cpanel/internal/cpanel"
)

type CronLineKey string

func (key *CronLineKey) UnmarshalJSON(data []byte) error {
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		if stringValue == "" {
			return fmt.Errorf("cron line key must not be empty")
		}
		*key = CronLineKey(stringValue)
		return nil
	}

	var numberValue json.Number
	if err := json.Unmarshal(data, &numberValue); err != nil {
		return fmt.Errorf("decode cron line key: %w", err)
	}
	if _, err := strconv.ParseUint(numberValue.String(), 10, 64); err != nil {
		return fmt.Errorf("decode cron line key integer: %w", err)
	}

	*key = CronLineKey(numberValue.String())
	return nil
}

type CronJobDataSourceModel struct {
	CpanelResult CronJobCpanelResultModel `json:"cpanelresult" tfsdk:"cpanelresult"`
}

type CronJobCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobDataSourceDataModel `json:"data" tfsdk:"data"`
}

type CronJobDetailsModel struct {
	Command string `json:"command" tfsdk:"command"`
	Minute  string `json:"minute" tfsdk:"minute"`
	Hour    string `json:"hour" tfsdk:"hour"`
	Day     string `json:"day" tfsdk:"day"`
	Weekday string `json:"weekday" tfsdk:"weekday"`
	Month   string `json:"month" tfsdk:"month"`
}

type CronJobDataSourceDataModel struct {
	CronJobDetailsModel
	LineKey         CronLineKey `json:"linekey" tfsdk:"linekey"`
	Line            int64       `json:"line" tfsdk:"line"`
	Value           string      `json:"value" tfsdk:"value"`
	Type            string      `json:"type" tfsdk:"type"`
	Key             string      `json:"key" tfsdk:"key"`
	Count           string      `json:"count" tfsdk:"count"`
	CommandNumber   int64       `json:"commandnumber" tfsdk:"commandnumber"`
	CommandHtmlSafe string      `json:"command_htmlsafe" tfsdk:"command_htmlsafe"`
	Reason          string      `json:"reason" tfsdk:"reason"`
	Result          bool        `json:"result" tfsdk:"result"`
}

type CronJobCreateModel struct {
	CronJobDetailsModel
}

type CronJobCreateDataSourceModel struct {
	CpanelResult CronJobCreateCpanelResultModel `json:"cpanelresult" tfsdk:"cpanelresult"`
}

type CronJobCreateCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobCreateDataSourceDataModel `json:"data" tfsdk:"data"`
}

type CronJobCreateDataSourceDataModel struct {
	LineKey CronLineKey `json:"linekey" tfsdk:"linekey"`
	CronJobCommonDataSourceDataModel
}

type CronJobUpdateModel struct {
	LineKey string `json:"linekey" tfsdk:"linekey"`
	CronJobDetailsModel
	Expected *CronJobDetailsModel `json:"-"`
}

type CronJobDeleteModel struct {
	LineKey  string               `json:"linekey" tfsdk:"linekey"`
	Expected *CronJobDetailsModel `json:"-"`
}

type CronJobDeleteDataSourceModel struct {
	CpanelResult CronJobDeleteCpanelResultModel `json:"cpanelresult" tfsdk:"cpanelresult"`
}

type CronJobDeleteCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobDeleteDataSourceDataModel `json:"data" tfsdk:"data"`
}

type CronJobDeleteDataSourceDataModel struct {
	CronJobCommonDataSourceDataModel
}

type CronJobCommonDataSourceDataModel struct {
	StatusMsg string `json:"statusmsg" tfsdk:"statusmsg"`
	Status    int64  `json:"status" tfsdk:"status"`
	Reason    string `json:"reason" tfsdk:"reason"`
	Result    int64  `json:"result" tfsdk:"result"`
}
