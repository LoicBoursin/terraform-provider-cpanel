package locale

import "terraform-provider-cpanel/internal/cpanel"

const (
	AccountIdentity      = "account"
	DirectionLeftToRight = "ltr"
	DirectionRightToLeft = "rtl"
)

type Locale struct {
	Code      string
	Direction string
	Encoding  string
	LocalName string
	Name      string
}

type attributesResponse struct {
	cpanel.UAPIDataSourceModel
	Data attributes `json:"data"`
}

type listResponse struct {
	cpanel.UAPIDataSourceModel
	Data []apiLocale `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
}

type attributes struct {
	Direction string `json:"direction"`
	Encoding  string `json:"encoding"`
	Locale    string `json:"locale"`
}

type apiLocale struct {
	Direction string `json:"direction"`
	LocalName string `json:"local_name"`
	Locale    string `json:"locale"`
	Name      string `json:"name"`
}
