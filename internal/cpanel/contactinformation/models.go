package contactinformation

import (
	"encoding/json"

	"terraform-provider-cpanel/internal/cpanel"
)

const AccountIdentity = "account"

type NotificationPreferences struct {
	Preferences  map[string]bool
	Descriptions map[string]string
}

func (p NotificationPreferences) Definition() map[string]bool {
	return clonePreferences(p.Preferences)
}

type notificationPreferencesResponse struct {
	cpanel.UAPIDataSourceModel
	Data []apiNotificationPreference `json:"data"`
}

type notificationPreferencesMutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type apiNotificationPreference struct {
	Name        string          `json:"name"`
	Enabled     json.RawMessage `json:"enabled"`
	Description string          `json:"descp"`
}

type notificationPreferencesRequest struct {
	Preferences map[string]int `json:"preferences"`
}

func clonePreferences(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for name, enabled := range source {
		result[name] = enabled
	}

	return result
}
