package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

const maxMySQLInventoryNameLength = 64

type inventoryResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type remoteHostInventoryResponse struct {
	CpanelResult struct {
		Data json.RawMessage `json:"data"`
	} `json:"cpanelresult"`
}

type restrictionsResponse struct {
	cpanel.UAPIDataSourceModel
	Data *Restrictions `json:"data"`
}

// ListDatabaseNames returns the complete MySQL database-name inventory.
func (c *Client) ListDatabaseNames(ctx context.Context) ([]string, error) {
	return c.listMySQLNames(
		ctx,
		operationListDatabases,
		"database",
		"database",
	)
}

// ListUserNames returns the complete MySQL username inventory.
func (c *Client) ListUserNames(ctx context.Context) ([]string, error) {
	return c.listMySQLNames(
		ctx,
		operationListUsers,
		"user",
		"user",
	)
}

// ListRemoteHostNames returns the complete normalized remote-host inventory.
func (c *Client) ListRemoteHostNames(ctx context.Context) ([]string, error) {
	response := remoteHostInventoryResponse{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleMysqlFE,
		operationListRemoteHosts,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	items, err := decodeRequiredJSONArray(
		response.CpanelResult.Data,
		"remote MySQL host inventory data",
	)
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		rawHost, err := decodeRequiredObjectString(
			item,
			"host",
			"remote MySQL host inventory item",
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel remote MySQL host inventory item %d: %w",
				index,
				err,
			)
		}
		host, err := NormalizeRemoteHost(rawHost)
		if err != nil {
			return nil, fmt.Errorf(
				"normalize cPanel remote MySQL host %q: %w",
				rawHost,
				err,
			)
		}
		if _, duplicate := seen[host]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate remote MySQL host %q after normalization",
				host,
			)
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}
	slices.Sort(hosts)

	return hosts, nil
}

func (c *Client) listMySQLNames(
	ctx context.Context,
	operation string,
	kind string,
	field string,
) ([]string, error) {
	response := inventoryResponse{}
	if err := c.executeReadOperation(
		ctx,
		operation,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectMySQLInventoryWarnings(
		fmt.Sprintf("MySQL %s inventory", kind),
		response.Warnings,
	); err != nil {
		return nil, err
	}

	items, err := decodeRequiredJSONArray(
		response.Data,
		fmt.Sprintf("MySQL %s inventory data", kind),
	)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index, item := range items {
		rawName, err := decodeRequiredObjectString(
			item,
			field,
			fmt.Sprintf("MySQL %s inventory item", kind),
		)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel MySQL %s inventory item %d: %w",
				kind,
				index,
				err,
			)
		}
		name, err := normalizeMySQLInventoryName(rawName, kind)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel MySQL %s inventory item %d: %w",
				kind,
				index,
				err,
			)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate MySQL %s name %q",
				kind,
				name,
			)
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	slices.Sort(names)

	return names, nil
}

func normalizeMySQLInventoryName(value string, kind string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf(
			"MySQL %s name must be a non-empty trimmed string",
			kind,
		)
	}
	if len(value) > maxMySQLInventoryNameLength {
		return "", fmt.Errorf(
			"MySQL %s name must not exceed %d ASCII characters",
			kind,
			maxMySQLInventoryNameLength,
		)
	}
	for _, character := range value {
		isLetter := character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z'
		isDigit := character >= '0' && character <= '9'
		if !isLetter && !isDigit && character != '_' {
			return "", fmt.Errorf(
				"MySQL %s name may contain only ASCII letters, numbers, and underscores",
				kind,
			)
		}
	}

	return value, nil
}

func decodeRequiredJSONArray(
	raw json.RawMessage,
	label string,
) ([]json.RawMessage, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("cPanel %s must be a non-null array", label)
	}

	items := make([]json.RawMessage, 0)
	if err := json.Unmarshal(value, &items); err != nil {
		return nil, fmt.Errorf(
			"decode cPanel %s as an array: %w",
			label,
			err,
		)
	}

	return items, nil
}

func decodeRequiredJSONObject(
	raw json.RawMessage,
	label string,
) (map[string]json.RawMessage, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s must be a non-null object", label)
	}

	object := map[string]json.RawMessage{}
	if err := json.Unmarshal(value, &object); err != nil {
		return nil, fmt.Errorf("decode %s as an object: %w", label, err)
	}

	return object, nil
}

func decodeRequiredObjectString(
	raw json.RawMessage,
	field string,
	label string,
) (string, error) {
	object, err := decodeRequiredJSONObject(raw, label)
	if err != nil {
		return "", err
	}
	value, exists := object[field]
	if !exists {
		return "", fmt.Errorf("%s is missing required field %q", label, field)
	}

	return decodeRequiredJSONString(value, field)
}

func decodeRequiredJSONString(
	raw json.RawMessage,
	field string,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("field %q must be a non-null string", field)
	}

	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", fmt.Errorf("field %q must be a string: %w", field, err)
	}

	return result, nil
}

func rejectMySQLInventoryWarnings(
	label string,
	warnings []string,
) error {
	if len(warnings) == 0 {
		return nil
	}

	return fmt.Errorf(
		"%s returned warnings and cannot be treated as complete: %s",
		label,
		strings.Join(warnings, "; "),
	)
}

func decodeRequiredPositiveJSONInteger(
	object map[string]json.RawMessage,
	field string,
) (int, error) {
	raw, exists := object[field]
	if !exists {
		return 0, fmt.Errorf(
			"MySQL restrictions are missing required field %q",
			field,
		)
	}
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf(
			"MySQL restriction %q must be a positive integer",
			field,
		)
	}

	var result int
	if err := json.Unmarshal(value, &result); err != nil {
		return 0, fmt.Errorf(
			"MySQL restriction %q must be a positive integer: %w",
			field,
			err,
		)
	}
	if result <= 0 {
		return 0, fmt.Errorf(
			"MySQL restriction %q must be greater than zero",
			field,
		)
	}

	return result, nil
}

func decodeMySQLPrefix(
	object map[string]json.RawMessage,
) (string, error) {
	raw, exists := object["prefix"]
	if !exists {
		return "", fmt.Errorf(
			"MySQL restrictions are missing required field %q",
			"prefix",
		)
	}
	value := bytes.TrimSpace(raw)
	if bytes.Equal(value, []byte("null")) {
		return "", nil
	}

	prefix, err := decodeRequiredJSONString(value, "prefix")
	if err != nil {
		return "", err
	}
	if prefix == "" {
		return "", nil
	}
	if _, err := normalizeMySQLInventoryName(prefix, "prefix"); err != nil {
		return "", err
	}
	if !strings.HasSuffix(prefix, "_") {
		return "", fmt.Errorf(
			"MySQL prefix must end with an underscore",
		)
	}

	return prefix, nil
}

func (r *Restrictions) UnmarshalJSON(data []byte) error {
	object, err := decodeRequiredJSONObject(
		data,
		"MySQL restrictions data",
	)
	if err != nil {
		return err
	}
	prefix, err := decodeMySQLPrefix(object)
	if err != nil {
		return err
	}
	maxDatabaseNameLength, err := decodeRequiredPositiveJSONInteger(
		object,
		"max_database_name_length",
	)
	if err != nil {
		return err
	}
	maxUsernameLength, err := decodeRequiredPositiveJSONInteger(
		object,
		"max_username_length",
	)
	if err != nil {
		return err
	}
	if len(prefix) >= maxDatabaseNameLength {
		return fmt.Errorf(
			"MySQL database name limit %d must allow at least one character after prefix %q",
			maxDatabaseNameLength,
			prefix,
		)
	}
	if len(prefix) >= maxUsernameLength {
		return fmt.Errorf(
			"MySQL username limit %d must allow at least one character after prefix %q",
			maxUsernameLength,
			prefix,
		)
	}

	*r = Restrictions{
		MaxUsernameLength:     maxUsernameLength,
		MaxDatabaseNameLength: maxDatabaseNameLength,
		Prefix:                prefix,
	}

	return nil
}
