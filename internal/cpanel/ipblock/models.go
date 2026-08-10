package ipblock

import "terraform-provider-cpanel/internal/cpanel"

type ListResponse struct {
	CpanelResult ListResult `json:"cpanelresult"`
}

type ListResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []BlockedAddress `json:"data"`
}

type BlockedAddress struct {
	Address string `json:"ip"`
	Range   string `json:"range"`
	Start   string `json:"start"`
	End     string `json:"end"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data []string `json:"data"`
}
