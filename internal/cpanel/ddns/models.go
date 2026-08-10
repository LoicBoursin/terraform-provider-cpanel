package ddns

import "terraform-provider-cpanel/internal/cpanel"

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Domain `json:"data"`
}

type CreateResponse struct {
	cpanel.UAPIDataSourceModel
	Data CreatedDomain `json:"data"`
}

type DeleteResponse struct {
	cpanel.UAPIDataSourceModel
	Data DeleteResult `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type RecreateResponse struct {
	cpanel.UAPIDataSourceModel
	Data RecreateResult `json:"data"`
}

type Domain struct {
	CreatedTime    int64    `json:"created_time"`
	Description    string   `json:"description"`
	Domain         string   `json:"domain"`
	ID             string   `json:"id"`
	IPv4           []string `json:"ipv4"`
	IPv6           []string `json:"ipv6"`
	LastRunTimes   []int64  `json:"last_run_times"`
	LastUpdateTime *int64   `json:"last_update_time"`
}

type CreatedDomain struct {
	CreatedTime int64  `json:"created_time"`
	ID          string `json:"id"`
}

type DeleteResult struct {
	Deleted int `json:"deleted"`
}

type RecreateResult struct {
	ID string `json:"id"`
}
