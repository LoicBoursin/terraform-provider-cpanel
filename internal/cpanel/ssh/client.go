package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

// Client reads public-only SSH keys stored by cPanel.
type Client struct {
	*cpanel.Client
}

// NewClient creates an SSH public-key client from the shared cPanel client.
func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

// List returns the complete public-key inventory. It always passes pub=1 and
// therefore never requests private-key metadata.
func (c *Client) List(ctx context.Context) ([]Metadata, error) {
	response := api2Response{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleSSH,
		operationListKeys,
		map[string]string{"pub": "1"},
		&response,
	); err != nil {
		return nil, err
	}

	return parseInventory(response.CpanelResult.Data)
}

// Get returns one fetched public key. It always passes pub=1 and validates the
// returned OpenSSH material locally.
func (c *Client) Get(
	ctx context.Context,
	name string,
) (*PublicKey, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	inventory, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(inventory), func(index int) bool {
		return inventory[index].Name >= name
	})
	if index == len(inventory) || inventory[index].Name != name {
		return nil, nil
	}

	return c.fetch(ctx, inventory[index])
}

func (c *Client) fetch(
	ctx context.Context,
	metadata Metadata,
) (*PublicKey, error) {
	response := api2Response{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleSSH,
		operationFetchKey,
		map[string]string{
			"name": metadata.Name,
			"pub":  "1",
		},
		&response,
	); err != nil {
		return nil, err
	}

	var values []apiFetchItem
	if err := decodeRequiredArray(
		response.CpanelResult.Data,
		&values,
		"fetched SSH public-key data",
	); err != nil {
		return nil, err
	}
	if len(values) != 1 {
		return nil, fmt.Errorf(
			"cPanel returned %d fetched SSH public keys for %q; expected exactly one",
			len(values),
			metadata.Name,
		)
	}

	filename, err := parseRequiredString(values[0].Name, "fetched key name")
	if err != nil {
		return nil, err
	}
	expectedFilename := metadata.Name + ".pub"
	if filename != expectedFilename {
		return nil, fmt.Errorf(
			"cPanel fetched SSH public-key filename %q for %q; expected %q",
			filename,
			metadata.Name,
			expectedFilename,
		)
	}
	publicKey, err := parseRequiredString(
		values[0].Key,
		"fetched public key",
	)
	if err != nil {
		return nil, err
	}
	parsed, err := ParsePublicKey(publicKey)
	if err != nil {
		return nil, fmt.Errorf(
			"validate fetched cPanel SSH public key %q: %w",
			metadata.Name,
			err,
		)
	}

	return &PublicKey{
		Metadata:          metadata,
		PublicKey:         parsed.PublicKey,
		FingerprintSHA256: parsed.FingerprintSHA256,
		ContentSHA256:     parsed.ContentSHA256,
		KeyType:           parsed.KeyType,
	}, nil
}

func parseInventory(raw json.RawMessage) ([]Metadata, error) {
	var values []apiListItem
	if err := decodeRequiredArray(
		raw,
		&values,
		"SSH public-key inventory data",
	); err != nil {
		return nil, err
	}

	result := make([]Metadata, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		metadata, err := metadataFromAPI(value)
		if err != nil {
			return nil, fmt.Errorf(
				"decode cPanel SSH public-key inventory item %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seen[metadata.Name]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate SSH public-key name %q",
				metadata.Name,
			)
		}
		seen[metadata.Name] = struct{}{}
		result = append(result, metadata)
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].Name < result[right].Name
	})

	return result, nil
}

func metadataFromAPI(value apiListItem) (Metadata, error) {
	name, err := parseRequiredString(value.Name, "inventory name")
	if err != nil {
		return Metadata{}, err
	}
	if err := ValidateName(name); err != nil {
		return Metadata{}, err
	}

	keyFilename, err := parseRequiredString(
		value.Key,
		"inventory key filename",
	)
	if err != nil {
		return Metadata{}, err
	}
	expectedFilename := name + ".pub"
	if keyFilename != expectedFilename {
		return Metadata{}, fmt.Errorf(
			"SSH public-key inventory filename is %q; expected %q",
			keyFilename,
			expectedFilename,
		)
	}

	file, err := parseRequiredString(value.File, "inventory file")
	if err != nil {
		return Metadata{}, err
	}
	if filepath.Base(file) != expectedFilename {
		return Metadata{}, fmt.Errorf(
			"SSH public-key inventory path %q does not end in %q",
			file,
			expectedFilename,
		)
	}

	authorized, err := parseAuthorization(
		value.Auth,
		value.AuthStatus,
	)
	if err != nil {
		return Metadata{}, err
	}

	createdAt, err := parseRequiredInt64(value.CTime, "inventory ctime")
	if err != nil {
		return Metadata{}, err
	}
	modifiedAt, err := parseRequiredInt64(value.MTime, "inventory mtime")
	if err != nil {
		return Metadata{}, err
	}
	if createdAt < 0 || modifiedAt < 0 {
		return Metadata{}, fmt.Errorf(
			"SSH public-key inventory timestamps must be non-negative",
		)
	}
	if len(value.HasPublic) != 0 &&
		string(value.HasPublic) != "null" {
		if _, err := parseFlexibleBool(value.HasPublic, "inventory haspub"); err != nil {
			return Metadata{}, err
		}
	}

	return Metadata{
		Name:       name,
		Authorized: authorized,
		CreatedAt:  createdAt,
		ModifiedAt: modifiedAt,
	}, nil
}

func parseAuthorization(
	authRaw json.RawMessage,
	statusRaw json.RawMessage,
) (bool, error) {
	signals := make([]bool, 0, 2)
	for _, signal := range []struct {
		label string
		raw   json.RawMessage
	}{
		{label: "inventory auth", raw: authRaw},
		{label: "inventory authstatus", raw: statusRaw},
	} {
		value, present, err := parseAuthorizationSignal(
			signal.raw,
			signal.label,
		)
		if err != nil {
			return false, err
		}
		if present {
			signals = append(signals, value)
		}
	}
	if len(signals) == 0 {
		return false, fmt.Errorf(
			"SSH public-key inventory has no authorization status",
		)
	}
	for _, signal := range signals[1:] {
		if signal != signals[0] {
			return false, fmt.Errorf(
				"SSH public-key authorization fields disagree",
			)
		}
	}

	return signals[0], nil
}

func parseAuthorizationSignal(
	raw json.RawMessage,
	label string,
) (bool, bool, error) {
	if len(raw) == 0 ||
		string(raw) == "null" ||
		string(raw) == `""` {
		return false, false, nil
	}
	if value, err := parseFlexibleBool(raw, label); err == nil {
		return value, true, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return false, false, fmt.Errorf(
			"decode SSH %s as authorization status",
			label,
		)
	}
	switch strings.ToLower(text) {
	case "authorized":
		return true, true, nil
	case "deauthorized", "unauthorized", "not authorized":
		return false, true, nil
	default:
		return false, false, fmt.Errorf(
			"SSH %s value %q is not an authorization status",
			label,
			text,
		)
	}
}

func decodeRequiredArray(
	raw json.RawMessage,
	output any,
	label string,
) error {
	if len(raw) == 0 || string(raw) == "null" {
		return fmt.Errorf("cPanel %s must be an array", label)
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode cPanel %s: %w", label, err)
	}

	return nil
}

func parseRequiredString(
	raw json.RawMessage,
	label string,
) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("SSH %s is required", label)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("decode SSH %s as string: %w", label, err)
	}
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf(
			"SSH %s must be a non-empty trimmed string",
			label,
		)
	}

	return value, nil
}

func parseRequiredInt64(
	raw json.RawMessage,
	label string,
) (int64, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, fmt.Errorf("SSH %s is required", label)
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		return integer, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, fmt.Errorf(
			"decode SSH %s as integer or decimal string",
			label,
		)
	}
	if text == "" || strings.TrimSpace(text) != text {
		return 0, fmt.Errorf(
			"SSH %s must be a non-empty decimal string",
			label,
		)
	}
	value, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf(
			"parse SSH %s decimal string %q: %w",
			label,
			text,
			err,
		)
	}

	return value, nil
}

func parseFlexibleBool(
	raw json.RawMessage,
	label string,
) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, fmt.Errorf("SSH %s is required", label)
	}
	var boolean bool
	if err := json.Unmarshal(raw, &boolean); err == nil {
		return boolean, nil
	}
	var integer int64
	if err := json.Unmarshal(raw, &integer); err == nil {
		switch integer {
		case 0:
			return false, nil
		case 1:
			return true, nil
		default:
			return false, fmt.Errorf(
				"SSH %s integer must be 0 or 1",
				label,
			)
		}
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return false, fmt.Errorf(
			"decode SSH %s as boolean, 0/1, or boolean string",
			label,
		)
	}
	switch strings.ToLower(text) {
	case "0", "false", "no":
		return false, nil
	case "1", "true", "yes":
		return true, nil
	default:
		return false, fmt.Errorf(
			"SSH %s string %q is not a boolean",
			label,
			text,
		)
	}
}
