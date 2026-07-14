package mimetype

import (
	"sort"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type ListResponse struct {
	cpanel.UAPIDataSourceModel
	Data []MIMEType `json:"data"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data any `json:"data"`
}

type MIMEType struct {
	Extension string `json:"extension"`
	Origin    string `json:"origin"`
	Type      string `json:"type"`
}

func (m MIMEType) Extensions() []string {
	extensions := strings.Fields(m.Extension)
	sort.Strings(extensions)

	return extensions
}

type Definition struct {
	Type       string
	Extensions []string
}

func (d Definition) Sorted() Definition {
	sorted := append([]string(nil), d.Extensions...)
	sort.Strings(sorted)
	d.Extensions = sorted

	return d
}
