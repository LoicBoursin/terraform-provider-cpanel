package redirect

import "terraform-provider-cpanel/internal/cpanel"

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []Redirect `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type Redirect struct {
	Destination   string `json:"destination"`
	DisplayDomain string `json:"displaydomain"`
	DisplaySource string `json:"displaysourceurl"`
	DocumentRoot  string `json:"docroot"`
	Domain        string `json:"domain"`
	Kind          string `json:"kind"`
	MatchWWW      int    `json:"matchwww"`
	Options       string `json:"opts"`
	Source        string `json:"source"`
	StatusCode    string `json:"statuscode"`
	TargetURL     string `json:"targeturl"`
	Type          string `json:"type"`
	URLDomain     string `json:"urldomain"`
	Wildcard      int    `json:"wildcard"`
}

type Definition struct {
	Domain      string
	Source      string
	Destination string
	Type        string
	WWWMode     string
	Wildcard    bool
}
