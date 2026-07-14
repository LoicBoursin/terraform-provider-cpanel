package directoryprivacy

import "terraform-provider-cpanel/internal/cpanel"

type Privacy struct {
	Directory         string
	AbsoluteDirectory string
	AuthName          string
	AuthType          string
	PasswordFile      string
	Protected         bool
}

type Definition struct {
	Directory string
	AuthName  string
}

type Response struct {
	cpanel.UAPIDataSourceModel
	Data ResponseData `json:"data"`
}

type ResponseData struct {
	AuthName     string `json:"auth_name"`
	AuthType     string `json:"auth_type"`
	PasswordFile string `json:"passwd_file"`
	Protected    int    `json:"protected"`
}
