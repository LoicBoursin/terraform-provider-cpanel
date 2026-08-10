package versioncontrol

import "terraform-provider-cpanel/internal/cpanel"

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []APIRepository `json:"data"`
}

type RepositoryResponse struct {
	cpanel.UAPIDataSourceModel
	Data APIRepository `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type API2FileOperationResponse struct {
	CpanelResult API2FileOperationResult `json:"cpanelresult"`
}

type API2FileOperationResult struct {
	cpanel.API2DataSourceCpanelResultModel
	Data []API2FileOperationItem `json:"data"`
}

type API2FileOperationItem struct {
	Destination string `json:"dest"`
	Error       string `json:"err"`
	Reason      string `json:"reason"`
	Result      int    `json:"result"`
	Source      string `json:"src"`
}

type APIRepository struct {
	Name              string            `json:"name"`
	RepositoryRoot    string            `json:"repository_root"`
	Type              string            `json:"type"`
	Branch            string            `json:"branch"`
	AvailableBranches []string          `json:"available_branches"`
	CloneURLs         CloneURLs         `json:"clone_urls"`
	SourceRepository  *SourceRepository `json:"source_repository"`
	Deployable        int               `json:"deployable"`
}

type CloneURLs struct {
	ReadOnly  []string `json:"read_only"`
	ReadWrite []string `json:"read_write"`
}

type SourceRepository struct {
	RemoteName string `json:"remote_name"`
	URL        string `json:"url,omitempty"`
}

type Repository struct {
	Name                 string
	RepositoryRoot       string
	AbsoluteRoot         string
	Type                 string
	Branch               string
	AvailableBranches    []string
	ReadOnlyCloneURLs    []string
	ReadWriteCloneURLs   []string
	SourceRepositoryName string
	SourceRepositoryURL  string
	Deployable           bool
}

type Definition struct {
	Name                string
	RepositoryRoot      string
	SourceRepositoryURL string
}
