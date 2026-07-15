package logmanager

import (
	"encoding/json"

	"terraform-provider-cpanel/internal/cpanel"
)

const AccountIdentity = "account"

type Settings struct {
	ArchiveLogs   bool
	PruneArchive  bool
	RetentionDays int64
	UsingDefault  bool
}

func (s Settings) Definition() Definition {
	retentionDays := s.RetentionDays
	if s.UsingDefault {
		retentionDays = -1
	}

	return Definition{
		ArchiveLogs:   s.ArchiveLogs,
		PruneArchive:  s.PruneArchive,
		RetentionDays: retentionDays,
	}
}

type Definition struct {
	ArchiveLogs   bool
	PruneArchive  bool
	RetentionDays int64
}

type settingsResponse struct {
	cpanel.UAPIDataSourceModel
	Data apiSettings `json:"data"`
}

type mutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiSettings struct {
	ArchiveLogs   json.RawMessage `json:"archive_logs"`
	PruneArchive  json.RawMessage `json:"prune_archive"`
	RetentionDays json.RawMessage `json:"retention_days"`
	UsingDefault  json.RawMessage `json:"using_default"`
}
