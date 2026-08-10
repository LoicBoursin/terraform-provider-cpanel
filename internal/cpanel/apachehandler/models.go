package apachehandler

import "terraform-provider-cpanel/internal/cpanel"

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Handler `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type Handler struct {
	Extension string `json:"extension"`
	Handler   string `json:"handler"`
	Origin    string `json:"origin"`
}

type Definition struct {
	Extension string
	Handler   string
}
