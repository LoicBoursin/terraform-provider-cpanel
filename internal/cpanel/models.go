package cpanel

type API2DataSourceCpanelResultModel struct {
	ApiVersion int                 `json:"apiversion" tfsdk:"apiversion"`
	Func       string              `json:"func" tfsdk:"func"`
	Event      API2DataSourceEvent `json:"event" tfsdk:"event"`
	Module     string              `json:"module" tfsdk:"module"`
}

type API2DataSourceEvent struct {
	Result int `json:"result" tfsdk:"result"`
}

type UAPIDataSourceModel struct {
	Errors   []string               `json:"errors" tfsdk:"errors"`
	Messages []string               `json:"messages" tfsdk:"messages"`
	Metadata UAPIDataSourceMetadata `json:"metadata" tfsdk:"metadata"`
	Status   int64                  `json:"status" tfsdk:"status"`
	Warnings []string               `json:"warnings" tfsdk:"warnings"`
}

type UAPIDataSourceMetadata struct {
	Transformed int64 `json:"transformed" tfsdk:"transformed"`
}
