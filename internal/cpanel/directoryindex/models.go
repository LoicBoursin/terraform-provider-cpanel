package directoryindex

import "terraform-provider-cpanel/internal/cpanel"

const (
	IndexTypeDisabled = "disabled"
	IndexTypeFancy    = "fancy"
	IndexTypeInherit  = "inherit"
	IndexTypeStandard = "standard"
)

type Index struct {
	Directory         string
	AbsoluteDirectory string
	Type              string
}

type Definition struct {
	Directory string
	Type      string
}

type IndexResponse struct {
	cpanel.UAPIDataSourceModel
	Data string `json:"data"`
}

type UserInformationResponse struct {
	cpanel.UAPIDataSourceModel
	Data UserInformation `json:"data"`
}

type UserInformation struct {
	Home string `json:"home"`
}

type FileListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []FileEntry `json:"data"`
}

type FileEntry struct {
	File     string `json:"file"`
	FullPath string `json:"fullpath"`
	Type     string `json:"type"`
}
