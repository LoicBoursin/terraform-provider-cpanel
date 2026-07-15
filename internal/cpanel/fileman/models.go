package fileman

import "encoding/json"

const (
	EntryTypeDirectory = "dir"
	EntryTypeFile      = "file"
)

type Entry struct {
	Path         string
	AbsolutePath string
	Type         string
	Permissions  string
	SizeBytes    int64
	CreatedAt    int64
	ModifiedAt   int64
}

type Directory struct {
	Entry Entry
}

type TextFile struct {
	Entry   Entry
	Content string
}

type userInformationResponse struct {
	Data struct {
		Home json.RawMessage `json:"home"`
	} `json:"data"`
}

type rawDataResponse struct {
	Data json.RawMessage `json:"data"`
}

type rawEntry struct {
	File        json.RawMessage `json:"file"`
	FullPath    json.RawMessage `json:"fullpath"`
	Type        json.RawMessage `json:"type"`
	Permissions json.RawMessage `json:"nicemode"`
	Size        json.RawMessage `json:"size"`
	CreatedAt   json.RawMessage `json:"ctime"`
	ModifiedAt  json.RawMessage `json:"mtime"`
}

type api2Response struct {
	CpanelResult struct {
		APIVersion int             `json:"apiversion"`
		Error      json.RawMessage `json:"error"`
		Function   string          `json:"func"`
		Module     string          `json:"module"`
		Data       json.RawMessage `json:"data"`
	} `json:"cpanelresult"`
}

type fileContentData struct {
	Path        json.RawMessage `json:"path"`
	Directory   json.RawMessage `json:"dir"`
	Filename    json.RawMessage `json:"filename"`
	Content     json.RawMessage `json:"content"`
	FromCharset json.RawMessage `json:"from_charset"`
	ToCharset   json.RawMessage `json:"to_charset"`
}

type saveContentData struct {
	Path        json.RawMessage `json:"path"`
	FromCharset json.RawMessage `json:"from_charset"`
	ToCharset   json.RawMessage `json:"to_charset"`
}
