package boxtrapper

import "encoding/json"

type Configuration struct {
	EnableAutoWhitelist    bool
	FromAddresses          string
	FromName               *string
	QueueDays              int64
	SpamScore              float64
	WhitelistByAssociation bool
}

type Settings struct {
	Account string
	Enabled bool
	Configuration
}

type Definition struct {
	Enabled                bool
	EnableAutoWhitelist    bool
	FromAddresses          string
	QueueDays              int64
	SpamScore              float64
	WhitelistByAssociation bool
}

func (s Settings) Definition() Definition {
	return Definition{
		Enabled:                s.Enabled,
		EnableAutoWhitelist:    s.EnableAutoWhitelist,
		FromAddresses:          s.FromAddresses,
		QueueDays:              s.QueueDays,
		SpamScore:              s.SpamScore,
		WhitelistByAssociation: s.WhitelistByAssociation,
	}
}

type accountInventoryEntry struct {
	Account string
	Enabled bool
}

type accountListResponse struct {
	Accounts []accountInventoryEntry
}

type statusResponse struct {
	Enabled bool
}

type configurationResponse struct {
	Configuration Configuration
}

type mutationResponse struct {
	Warnings []string
}

type uapiEnvelope struct {
	Data     json.RawMessage
	Warnings []string
}
