package cpanel

type API2DataSourceCpanelResultModel struct {
	ApiVersion int                 `tfsdk:"apiversion"`
	Func       string              `tfsdk:"func"`
	Event      API2DataSourceEvent `tfsdk:"event"`
	Module     string              `tfsdk:"module"`
}

type API2DataSourceEvent struct {
	Result int `tfsdk:"result"`
}

type UAPIDataSourceModel struct {
	Errors   []string               `tfsdk:"errors"`
	Messages []string               `tfsdk:"messages"`
	Metadata UAPIDataSourceMetadata `tfsdk:"metadata"`
	Status   int64                  `tfsdk:"status"`
	Warnings []string               `tfsdk:"warnings"`
}

type UAPIDataSourceMetadata struct {
	Transformed int64 `tfsdk:"transformed"`
}
