package cron

import "terraform-provider-cpanel/internal/cpanel"

type CronJobDataSourceModel struct {
	CpanelResult CronJobCpanelResultModel `tfsdk:"cpanelresult"`
}

type CronJobCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobDataSourceDataModel `tfsdk:"data"`
}

type CronJobDetailsModel struct {
	Command string `tfsdk:"command"`
	Minute  string `tfsdk:"minute"`
	Hour    string `tfsdk:"hour"`
	Day     string `tfsdk:"day"`
	Weekday string `tfsdk:"weekday"`
	Month   string `tfsdk:"month"`
}

type CronJobDataSourceDataModel struct {
	CronJobDetailsModel
	LineKey         int64  `tfsdk:"linekey"`
	Value           string `tfsdk:"value"`
	Type            string `tfsdk:"type"`
	Key             string `tfsdk:"key"`
	Count           string `tfsdk:"count"`
	CommandNumber   int64  `tfsdk:"commandnumber"`
	CommandHtmlSafe string `tfsdk:"command_htmlsafe"`
	Reason          string `tfsdk:"reason"`
	Result          bool   `tfsdk:"result"`
}

type CronJobCreateModel struct {
	CronJobDetailsModel
}

type CronJobCreateDataSourceModel struct {
	CpanelResult CronJobCreateCpanelResultModel `tfsdk:"cpanelresult"`
}

type CronJobCreateCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobCreateDataSourceDataModel `tfsdk:"data"`
}

type CronJobCreateDataSourceDataModel struct {
	LineKey int64 `tfsdk:"linekey"`
	CronJobCommonDataSourceDataModel
}

type CronJobUpdateModel struct {
	LineKey int64 `tfsdk:"linekey"`
	CronJobDetailsModel
}

type CronJobDeleteModel struct {
	LineKey int64 `tfsdk:"linekey"`
}

type CronJobDeleteDataSourceModel struct {
	CpanelResult CronJobDeleteCpanelResultModel `tfsdk:"cpanelresult"`
}

type CronJobDeleteCpanelResultModel struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []CronJobDeleteDataSourceDataModel `tfsdk:"data"`
}

type CronJobDeleteDataSourceDataModel struct {
	CronJobCommonDataSourceDataModel
}

type CronJobCommonDataSourceDataModel struct {
	StatusMsg string `tfsdk:"statusmsg"`
	Status    int64  `tfsdk:"status"`
	Reason    string `tfsdk:"reason"`
	Result    int64  `tfsdk:"result"`
}
