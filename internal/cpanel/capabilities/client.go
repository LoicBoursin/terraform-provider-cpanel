package capabilities

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

var featureNamePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) Get(ctx context.Context) (*AccountCapabilities, error) {
	features, err := c.getFeatures(ctx)
	if err != nil {
		return nil, err
	}
	version, err := c.getVersion(ctx)
	if err != nil {
		return nil, err
	}
	userInformation, err := c.getUserInformation(ctx)
	if err != nil {
		return nil, err
	}

	return &AccountCapabilities{
		Version:       version,
		Username:      userInformation.Username,
		HomeDirectory: userInformation.Home,
		PrimaryDomain: userInformation.PrimaryDomain,
		Plan:          userInformation.Plan,
		Theme:         userInformation.Theme,
		Shell:         userInformation.Shell,
		Features:      features,
		Limits: map[string]string{
			"addon_domains":            userInformation.MaximumAddonDomains,
			"databases":                userInformation.MaximumDatabases,
			"defer_fail_percentage":    userInformation.MaximumDeferFailPercentage,
			"email_account_disk_quota": userInformation.MaximumEmailAccountDiskQuota,
			"emails_per_hour":          userInformation.MaximumEmailsPerHour,
			"ftp_accounts":             userInformation.MaximumFTPAccounts,
			"mail_accounts":            userInformation.MaximumMailAccounts,
			"mailing_lists":            userInformation.MaximumMailingLists,
			"parked_domains":           userInformation.MaximumParkedDomains,
			"passenger_apps":           userInformation.MaximumPassengerApps,
			"subdomains":               userInformation.MaximumSubdomains,
			"team_users":               userInformation.MaximumTeamUsers,
		},
	}, nil
}

func (c *Client) getFeatures(ctx context.Context) (map[string]bool, error) {
	response := featureListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleFeatures,
		operationListFeatures,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, fmt.Errorf("cPanel returned an empty feature inventory")
	}

	features := make(map[string]bool, len(response.Data))
	for name, rawValue := range response.Data {
		if !featureNamePattern.MatchString(name) {
			return nil, fmt.Errorf(
				"cPanel returned invalid feature name %q",
				name,
			)
		}
		enabled, err := parseFeatureFlag(rawValue)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel feature %q: %w",
				name,
				err,
			)
		}
		features[name] = enabled
	}

	return features, nil
}

func (c *Client) getVersion(ctx context.Context) (string, error) {
	response := statsResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleStatsBar,
		operationGetStats,
		map[string]string{"display": "cpanelversion"},
		&response,
	); err != nil {
		return "", err
	}

	version := ""
	for _, stat := range response.Data {
		if stat.Name != "cpanelversion" {
			continue
		}
		if version != "" {
			return "", fmt.Errorf(
				"cPanel returned duplicate cpanelversion statistics",
			)
		}
		version = strings.TrimSpace(stat.Value)
	}
	if version == "" {
		return "", fmt.Errorf(
			"cPanel did not return the cpanelversion statistic",
		)
	}

	return version, nil
}

func (c *Client) getUserInformation(
	ctx context.Context,
) (*apiUserInformation, error) {
	response := userInformationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleVariables,
		operationGetUserInformation,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	required := map[string]string{
		"home":   response.Data.Home,
		"domain": response.Data.PrimaryDomain,
		"user":   response.Data.Username,
	}
	for field, value := range required {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf(
				"cPanel user information returned an empty %s field",
				field,
			)
		}
	}
	if !strings.HasPrefix(response.Data.Home, "/") {
		return nil, fmt.Errorf(
			"cPanel user information returned non-absolute home %q",
			response.Data.Home,
		)
	}

	return &response.Data, nil
}

func parseFeatureFlag(rawValue []byte) (bool, error) {
	value := bytes.TrimSpace(rawValue)
	switch string(value) {
	case "0", `"0"`, "false":
		return false, nil
	case "1", `"1"`, "true":
		return true, nil
	default:
		return false, fmt.Errorf(
			"expected 0 or 1, got %q",
			value,
		)
	}
}
