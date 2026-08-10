package ssh

import (
	"encoding/json"
)

const (
	operationListKeys = "listkeys"
	operationFetchKey = "fetchkey"
)

// Metadata is the public-key inventory metadata returned by cPanel.
type Metadata struct {
	Name       string
	Authorized bool
	CreatedAt  int64
	ModifiedAt int64
}

// PublicKey is one fetched and locally validated OpenSSH public key.
type PublicKey struct {
	Metadata
	PublicKey         string
	FingerprintSHA256 string
	ContentSHA256     string
	KeyType           string
}

// ParsedPublicKey is the canonical identity of one OpenSSH public key.
type ParsedPublicKey struct {
	PublicKey         string
	FingerprintSHA256 string
	ContentSHA256     string
	KeyType           string
}

type api2Response struct {
	CpanelResult struct {
		Data json.RawMessage `json:"data"`
	} `json:"cpanelresult"`
}

type apiListItem struct {
	Auth       json.RawMessage `json:"auth"`
	AuthAction json.RawMessage `json:"authaction"`
	AuthStatus json.RawMessage `json:"authstatus"`
	CTime      json.RawMessage `json:"ctime"`
	File       json.RawMessage `json:"file"`
	HasPublic  json.RawMessage `json:"haspub"`
	Key        json.RawMessage `json:"key"`
	MTime      json.RawMessage `json:"mtime"`
	Name       json.RawMessage `json:"name"`
}

type apiFetchItem struct {
	Key  json.RawMessage `json:"key"`
	Name json.RawMessage `json:"name"`
}
