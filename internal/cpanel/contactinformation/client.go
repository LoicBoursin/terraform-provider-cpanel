package contactinformation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

var notificationPreferenceNamePattern = regexp.MustCompile(
	`^notify_[a-z0-9_]+$`,
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) GetNotificationPreferences(
	ctx context.Context,
) (*NotificationPreferences, error) {
	response := notificationPreferencesResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleContactInformation,
		operationGetNotificationPreferences,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, fmt.Errorf(
			"cPanel returned no account notification preferences",
		)
	}

	preferences := make(map[string]bool, len(response.Data))
	descriptions := make(map[string]string, len(response.Data))
	for _, item := range response.Data {
		if err := validatePreferenceName(item.Name); err != nil {
			return nil, err
		}
		if _, exists := preferences[item.Name]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate notification preference %q",
				item.Name,
			)
		}
		enabled, err := parseBooleanFlag(item.Name, item.Enabled)
		if err != nil {
			return nil, err
		}
		preferences[item.Name] = enabled
		descriptions[item.Name] = item.Description
	}

	return &NotificationPreferences{
		Preferences:  preferences,
		Descriptions: descriptions,
	}, nil
}

func (c *Client) SetNotificationPreferences(
	ctx context.Context,
	definition map[string]bool,
) (*NotificationPreferences, error) {
	if err := ValidateDefinition(definition); err != nil {
		return nil, err
	}

	current, err := c.GetNotificationPreferences(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"read notification preferences before mutation: %w",
			err,
		)
	}
	if err := validateAvailablePreferences(*current, definition); err != nil {
		return nil, err
	}

	requestPreferences := make(map[string]int, len(definition))
	for name, enabled := range definition {
		if enabled {
			requestPreferences[name] = 1
		} else {
			requestPreferences[name] = 0
		}
	}
	response := notificationPreferencesMutationResponse{}
	if err := c.ExecuteUAPIOperationJSON(
		ctx,
		cpanel.ModuleContactInformation,
		operationSetNotificationPreferences,
		notificationPreferencesRequest{Preferences: requestPreferences},
		&response,
	); err != nil {
		return nil, err
	}

	actual, err := c.GetNotificationPreferences(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"read notification preferences after mutation: %w",
			err,
		)
	}
	if err := verifyPreferences(*actual, definition); err != nil {
		return nil, err
	}

	return actual, nil
}

func ValidateDefinition(definition map[string]bool) error {
	if len(definition) == 0 {
		return fmt.Errorf(
			"notification preferences must contain at least one setting",
		)
	}
	for name := range definition {
		if err := validatePreferenceName(name); err != nil {
			return err
		}
	}

	return nil
}

func PreferencesMatchDefinition(
	preferences NotificationPreferences,
	definition map[string]bool,
) bool {
	return verifyPreferences(preferences, definition) == nil
}

func validateAvailablePreferences(
	current NotificationPreferences,
	definition map[string]bool,
) error {
	currentNames := sortedPreferenceNames(current.Preferences)
	targetNames := sortedPreferenceNames(definition)
	if !slices.Equal(currentNames, targetNames) {
		return fmt.Errorf(
			"configured notification preference names %q do not exactly match the cPanel account names %q",
			strings.Join(targetNames, ", "),
			strings.Join(currentNames, ", "),
		)
	}

	return nil
}

func verifyPreferences(
	actual NotificationPreferences,
	expected map[string]bool,
) error {
	if err := validateAvailablePreferences(actual, expected); err != nil {
		return err
	}
	for _, name := range sortedPreferenceNames(expected) {
		if actual.Preferences[name] != expected[name] {
			return fmt.Errorf(
				"cPanel notification preference %q is %t after mutation; expected %t",
				name,
				actual.Preferences[name],
				expected[name],
			)
		}
	}

	return nil
}

func validatePreferenceName(name string) error {
	if !notificationPreferenceNamePattern.MatchString(name) {
		return fmt.Errorf(
			"invalid cPanel notification preference name %q",
			name,
		)
	}

	return nil
}

func sortedPreferenceNames(preferences map[string]bool) []string {
	names := make([]string, 0, len(preferences))
	for name := range preferences {
		names = append(names, name)
	}
	slices.Sort(names)

	return names
}

func parseBooleanFlag(
	name string,
	raw json.RawMessage,
) (bool, error) {
	value := bytes.TrimSpace(raw)
	switch string(value) {
	case "0", `"0"`:
		return false, nil
	case "1", `"1"`:
		return true, nil
	default:
		return false, fmt.Errorf(
			"cPanel returned invalid enabled flag %q for notification preference %q; expected 0 or 1",
			value,
			name,
		)
	}
}
