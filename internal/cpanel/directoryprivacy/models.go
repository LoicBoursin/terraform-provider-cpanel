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

type UserListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []string `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type ResponseData struct {
	AuthName     string `json:"auth_name"`
	AuthType     string `json:"auth_type"`
	PasswordFile string `json:"passwd_file"`
	Protected    int    `json:"protected"`
}

type User struct {
	Directory         string
	AbsoluteDirectory string
	Username          string
}

type UserDefinition struct {
	Directory string
	Username  string
	Password  string
}
