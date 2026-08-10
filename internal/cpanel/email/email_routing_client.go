package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type RoutingMode string

const (
	RoutingModeAuto   RoutingMode = "auto"
	RoutingModeBackup RoutingMode = "backup"
	RoutingModeLocal  RoutingMode = "local"
	RoutingModeRemote RoutingMode = "remote"
)

const routingWireModeSecondary = "secondary"

type RoutingDefinition struct {
	Domain string
	Mode   RoutingMode
}

type RoutingMXEntry struct {
	Domain    string
	Exchanger string
	Priority  int64
	Order     int64
}

type Routing struct {
	Domain           string
	Mode             RoutingMode
	DetectedMode     RoutingMode
	PrimaryExchanger *string
	Entries          []RoutingMXEntry
	AlwaysAccept     bool
	Local            bool
	Remote           bool
	Backup           bool
}

type routingListResponse struct {
	cpanel.UAPIDataSourceModel
	Data json.RawMessage `json:"data"`
}

type routingMutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data apiRoutingMutation `json:"data"`
}

type apiRouting struct {
	AlwaysAccept json.RawMessage `json:"alwaysaccept"`
	DetectedMode json.RawMessage `json:"detected"`
	Domain       json.RawMessage `json:"domain"`
	Entries      json.RawMessage `json:"entries"`
	Local        json.RawMessage `json:"local"`
	Exchanger    json.RawMessage `json:"mx"`
	Mode         json.RawMessage `json:"mxcheck"`
	Remote       json.RawMessage `json:"remote"`
	Secondary    json.RawMessage `json:"secondary"`
	Status       json.RawMessage `json:"status"`
	StatusMsg    json.RawMessage `json:"statusmsg"`
}

type apiRoutingMXEntry struct {
	Domain     json.RawMessage `json:"domain"`
	EntryCount json.RawMessage `json:"entrycount"`
	Exchanger  json.RawMessage `json:"mx"`
	Priority   json.RawMessage `json:"priority"`
	Row        json.RawMessage `json:"row"`
}

type apiRoutingMutation struct {
	CheckMX   apiRoutingCheckMX `json:"checkmx"`
	Detected  json.RawMessage   `json:"detected"`
	Local     json.RawMessage   `json:"local"`
	Mode      json.RawMessage   `json:"mxcheck"`
	Remote    json.RawMessage   `json:"remote"`
	Results   json.RawMessage   `json:"results"`
	Secondary json.RawMessage   `json:"secondary"`
	Status    json.RawMessage   `json:"status"`
	StatusMsg json.RawMessage   `json:"statusmsg"`
}

type apiRoutingCheckMX struct {
	Changed     json.RawMessage `json:"changed"`
	Detected    json.RawMessage `json:"detected"`
	IsPrimary   json.RawMessage `json:"isprimary"`
	IsSecondary json.RawMessage `json:"issecondary"`
	Local       json.RawMessage `json:"local"`
	Mode        json.RawMessage `json:"mxcheck"`
	Remote      json.RawMessage `json:"remote"`
	Secondary   json.RawMessage `json:"secondary"`
	Warnings    []string        `json:"warnings"`
}

func (c *Client) ListRoutings(
	ctx context.Context,
) ([]Routing, error) {
	response := routingListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListMXs,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectEmailInventoryWarnings(
		"email routing inventory",
		response.Warnings,
	); err != nil {
		return nil, err
	}
	rows, err := decodeEmailInventoryRows[apiRouting](
		response.Data,
		"email routing inventory",
	)
	if err != nil {
		return nil, err
	}

	routings := make([]Routing, 0, len(rows))
	seenDomains := make(map[string]struct{}, len(rows))
	for index, apiValue := range rows {
		routing, err := routingFromAPI(apiValue)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid email routing entry at index %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seenDomains[routing.Domain]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate email routing entries for domain %q",
				routing.Domain,
			)
		}
		seenDomains[routing.Domain] = struct{}{}
		routings = append(routings, routing)
	}
	sort.Slice(routings, func(left, right int) bool {
		return routings[left].Domain < routings[right].Domain
	})

	return routings, nil
}

func (c *Client) GetRouting(
	ctx context.Context,
	domain string,
) (*Routing, error) {
	if err := validateRoutingDomain(domain); err != nil {
		return nil, err
	}

	routings, err := c.ListRoutings(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(routings), func(index int) bool {
		return routings[index].Domain >= domain
	})
	if index == len(routings) || routings[index].Domain != domain {
		return nil, nil
	}

	routing := routings[index]

	return &routing, nil
}

func (c *Client) SetRouting(
	ctx context.Context,
	definition RoutingDefinition,
) (*Routing, []string, error) {
	if err := ValidateRoutingDefinition(definition); err != nil {
		return nil, nil, err
	}
	wireMode, err := routingModeToWire(definition.Mode)
	if err != nil {
		return nil, nil, err
	}

	response := routingMutationResponse{}
	if err := c.executeMutation(
		ctx,
		operationSetAlwaysAccept,
		map[string]string{
			"domain":  definition.Domain,
			"mxcheck": wireMode,
		},
		&response,
	); err != nil {
		return nil, nil, err
	}
	warnings, err := validateRoutingMutationResponse(
		response,
		definition.Mode,
	)
	if err != nil {
		return nil, nil, err
	}

	actual, err := c.GetRouting(ctx, definition.Domain)
	if err != nil {
		return nil, warnings, fmt.Errorf(
			"read email routing after mutation: %w",
			err,
		)
	}
	if actual == nil {
		return nil, warnings, fmt.Errorf(
			"cPanel email routing for domain %q disappeared after mutation",
			definition.Domain,
		)
	}
	if !RoutingMatchesDefinition(*actual, definition) {
		return nil, warnings, fmt.Errorf(
			"cPanel email routing mode for domain %q is %q after mutation; expected %q",
			definition.Domain,
			actual.Mode,
			definition.Mode,
		)
	}

	return actual, warnings, nil
}

func ValidateRoutingDefinition(definition RoutingDefinition) error {
	if err := validateRoutingDomain(definition.Domain); err != nil {
		return err
	}
	if !validRoutingMode(definition.Mode) {
		return fmt.Errorf(
			"email routing mode must be auto, backup, local, or remote, got %q",
			definition.Mode,
		)
	}

	return nil
}

func RoutingMatchesDefinition(
	routing Routing,
	definition RoutingDefinition,
) bool {
	return routing.Domain == definition.Domain &&
		routing.Mode == definition.Mode
}

func routingFromAPI(value apiRouting) (Routing, error) {
	domain, err := parseRequiredRoutingString("domain", value.Domain)
	if err != nil {
		return Routing{}, err
	}
	if err := validateRoutingDomain(domain); err != nil {
		return Routing{}, err
	}

	status, err := parseRoutingInteger("status", value.Status, 0)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	if status != 1 {
		return Routing{}, fmt.Errorf(
			"domain %q returned row status %d; expected 1",
			domain,
			status,
		)
	}
	statusMessage, err := parseRequiredRoutingString(
		"statusmsg",
		value.StatusMsg,
	)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	if statusMessage == "" {
		return Routing{}, fmt.Errorf(
			"domain %q returned an empty routing status message",
			domain,
		)
	}

	mode, err := parseRoutingMode("mxcheck", value.Mode)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	detectedMode, err := parseRoutingMode("detected", value.DetectedMode)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	primaryExchanger, err := parseNullableRoutingString(
		"mx",
		value.Exchanger,
	)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	alwaysAccept, err := parseRoutingFlag(
		"alwaysaccept",
		value.AlwaysAccept,
	)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	local, err := parseRoutingFlag("local", value.Local)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	remote, err := parseRoutingFlag("remote", value.Remote)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	backup, err := parseRoutingFlag("secondary", value.Secondary)
	if err != nil {
		return Routing{}, fmt.Errorf("domain %q: %w", domain, err)
	}
	if err := validateRoutingFlags(
		domain,
		detectedMode,
		local,
		remote,
		backup,
	); err != nil {
		return Routing{}, err
	}

	entries, err := parseRoutingMXEntries(domain, value.Entries)
	if err != nil {
		return Routing{}, err
	}
	if len(entries) == 0 {
		if primaryExchanger != nil {
			return Routing{}, fmt.Errorf(
				"domain %q returned primary exchanger %q without MX entries",
				domain,
				*primaryExchanger,
			)
		}
	} else {
		if primaryExchanger == nil {
			return Routing{}, fmt.Errorf(
				"domain %q returned MX entries without a primary exchanger",
				domain,
			)
		}
		if entries[0].Exchanger != *primaryExchanger {
			return Routing{}, fmt.Errorf(
				"domain %q primary exchanger %q does not match highest-priority MX entry %q",
				domain,
				*primaryExchanger,
				entries[0].Exchanger,
			)
		}
	}

	return Routing{
		Domain:           domain,
		Mode:             mode,
		DetectedMode:     detectedMode,
		PrimaryExchanger: primaryExchanger,
		Entries:          entries,
		AlwaysAccept:     alwaysAccept,
		Local:            local,
		Remote:           remote,
		Backup:           backup,
	}, nil
}

func parseRoutingMXEntries(
	domain string,
	raw json.RawMessage,
) ([]RoutingMXEntry, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 ||
		bytes.Equal(value, []byte("null")) ||
		value[0] != '[' {
		return nil, fmt.Errorf(
			"domain %q returned invalid MX entry inventory",
			domain,
		)
	}

	var apiEntries []apiRoutingMXEntry
	if err := json.Unmarshal(value, &apiEntries); err != nil {
		return nil, fmt.Errorf(
			"domain %q: decode MX entries: %w",
			domain,
			err,
		)
	}

	entries := make([]RoutingMXEntry, 0, len(apiEntries))
	seenOrders := make(map[int64]struct{}, len(apiEntries))
	seenMX := make(map[string]struct{}, len(apiEntries))
	for index, apiEntry := range apiEntries {
		entryDomain, err := parseRequiredRoutingString(
			"entry domain",
			apiEntry.Domain,
		)
		if err != nil {
			return nil, routingMXEntryError(domain, index, err)
		}
		if entryDomain != domain {
			return nil, routingMXEntryError(
				domain,
				index,
				fmt.Errorf(
					"entry domain is %q; expected %q",
					entryDomain,
					domain,
				),
			)
		}
		exchanger, err := parseRequiredRoutingString(
			"entry mx",
			apiEntry.Exchanger,
		)
		if err != nil {
			return nil, routingMXEntryError(domain, index, err)
		}
		priority, err := parseRoutingInteger(
			"entry priority",
			apiEntry.Priority,
			0,
		)
		if err != nil {
			return nil, routingMXEntryError(domain, index, err)
		}
		order, err := parseRoutingInteger(
			"entrycount",
			apiEntry.EntryCount,
			1,
		)
		if err != nil {
			return nil, routingMXEntryError(domain, index, err)
		}
		row, err := parseRequiredRoutingString("row", apiEntry.Row)
		if err != nil {
			return nil, routingMXEntryError(domain, index, err)
		}
		expectedRow := "even"
		if order%2 == 1 {
			expectedRow = "odd"
		}
		if row != expectedRow {
			return nil, routingMXEntryError(
				domain,
				index,
				fmt.Errorf(
					"row is %q for order %d; expected %q",
					row,
					order,
					expectedRow,
				),
			)
		}
		if _, duplicate := seenOrders[order]; duplicate {
			return nil, routingMXEntryError(
				domain,
				index,
				fmt.Errorf("duplicate entry order %d", order),
			)
		}
		seenOrders[order] = struct{}{}
		mxIdentity := fmt.Sprintf("%d\x00%s", priority, exchanger)
		if _, duplicate := seenMX[mxIdentity]; duplicate {
			return nil, routingMXEntryError(
				domain,
				index,
				fmt.Errorf(
					"duplicate MX entry %q at priority %d",
					exchanger,
					priority,
				),
			)
		}
		seenMX[mxIdentity] = struct{}{}
		entries = append(entries, RoutingMXEntry{
			Domain:    entryDomain,
			Exchanger: exchanger,
			Priority:  priority,
			Order:     order,
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].Priority != entries[right].Priority {
			return entries[left].Priority < entries[right].Priority
		}
		if entries[left].Exchanger != entries[right].Exchanger {
			return entries[left].Exchanger < entries[right].Exchanger
		}

		return entries[left].Order < entries[right].Order
	})

	return entries, nil
}

func validateRoutingMutationResponse(
	response routingMutationResponse,
	expectedMode RoutingMode,
) ([]string, error) {
	wireMode, err := routingModeToWire(expectedMode)
	if err != nil {
		return nil, err
	}

	status, err := parseRoutingInteger(
		"mutation status",
		response.Data.Status,
		0,
	)
	if err != nil {
		return nil, err
	}
	if status != 1 {
		return nil, fmt.Errorf(
			"cPanel email routing mutation returned status %d; expected 1",
			status,
		)
	}
	for name, raw := range map[string]json.RawMessage{
		"mutation results":   response.Data.Results,
		"mutation statusmsg": response.Data.StatusMsg,
	} {
		if _, err := parseRequiredRoutingString(name, raw); err != nil {
			return nil, err
		}
	}
	outerMode, err := parseRequiredRoutingString(
		"mutation mxcheck",
		response.Data.Mode,
	)
	if err != nil {
		return nil, err
	}
	if outerMode != wireMode {
		return nil, fmt.Errorf(
			"cPanel email routing mutation returned mxcheck %q; expected %q",
			outerMode,
			wireMode,
		)
	}
	outerDetected, err := parseRoutingMode(
		"mutation detected",
		response.Data.Detected,
	)
	if err != nil {
		return nil, err
	}
	outerLocal, err := parseRoutingFlag(
		"mutation local",
		response.Data.Local,
	)
	if err != nil {
		return nil, err
	}
	outerRemote, err := parseRoutingFlag(
		"mutation remote",
		response.Data.Remote,
	)
	if err != nil {
		return nil, err
	}
	outerBackup, err := parseRoutingFlag(
		"mutation secondary",
		response.Data.Secondary,
	)
	if err != nil {
		return nil, err
	}
	if err := validateRoutingFlags(
		"mutation",
		outerDetected,
		outerLocal,
		outerRemote,
		outerBackup,
	); err != nil {
		return nil, err
	}

	if _, err := parseRoutingFlag(
		"mutation changed",
		response.Data.CheckMX.Changed,
	); err != nil {
		return nil, err
	}
	checkMode, err := parseRequiredRoutingString(
		"mutation checkmx mxcheck",
		response.Data.CheckMX.Mode,
	)
	if err != nil {
		return nil, err
	}
	if checkMode != wireMode {
		return nil, fmt.Errorf(
			"cPanel email routing check returned mxcheck %q; expected %q",
			checkMode,
			wireMode,
		)
	}
	checkDetected, err := parseRoutingMode(
		"mutation checkmx detected",
		response.Data.CheckMX.Detected,
	)
	if err != nil {
		return nil, err
	}
	if checkDetected != outerDetected {
		return nil, fmt.Errorf(
			"cPanel email routing mutation detected modes disagree: %q and %q",
			outerDetected,
			checkDetected,
		)
	}
	checkLocal, err := parseRoutingFlag(
		"mutation checkmx local",
		response.Data.CheckMX.Local,
	)
	if err != nil {
		return nil, err
	}
	checkRemote, err := parseRoutingFlag(
		"mutation checkmx remote",
		response.Data.CheckMX.Remote,
	)
	if err != nil {
		return nil, err
	}
	checkBackup, err := parseRoutingFlag(
		"mutation checkmx secondary",
		response.Data.CheckMX.Secondary,
	)
	if err != nil {
		return nil, err
	}
	if checkLocal != outerLocal ||
		checkRemote != outerRemote ||
		checkBackup != outerBackup {
		return nil, fmt.Errorf(
			"cPanel email routing mutation flags disagree between result and checkmx",
		)
	}
	isPrimary, err := parseRoutingFlag(
		"mutation checkmx isprimary",
		response.Data.CheckMX.IsPrimary,
	)
	if err != nil {
		return nil, err
	}
	isSecondary, err := parseRoutingFlag(
		"mutation checkmx issecondary",
		response.Data.CheckMX.IsSecondary,
	)
	if err != nil {
		return nil, err
	}
	if isPrimary != checkLocal ||
		isSecondary != checkBackup {
		return nil, fmt.Errorf(
			"cPanel email routing mutation primary flags disagree with the detected mode",
		)
	}

	warnings := append([]string{}, response.Warnings...)
	warnings = append(warnings, response.Data.CheckMX.Warnings...)
	for index, warning := range warnings {
		if warning == "" || strings.TrimSpace(warning) != warning {
			return nil, fmt.Errorf(
				"cPanel email routing mutation returned invalid warning at index %d",
				index,
			)
		}
	}

	return warnings, nil
}

func validateRoutingFlags(
	domain string,
	detectedMode RoutingMode,
	local bool,
	remote bool,
	backup bool,
) error {
	expectedLocal := detectedMode == RoutingModeLocal
	expectedRemote := detectedMode == RoutingModeRemote
	expectedBackup := detectedMode == RoutingModeBackup
	if local != expectedLocal ||
		remote != expectedRemote ||
		backup != expectedBackup {
		return fmt.Errorf(
			"%s returned routing flags local=%t remote=%t secondary=%t inconsistent with detected mode %q",
			domain,
			local,
			remote,
			backup,
			detectedMode,
		)
	}

	return nil
}

func parseRoutingMode(
	name string,
	raw json.RawMessage,
) (RoutingMode, error) {
	wireMode, err := parseRequiredRoutingString(name, raw)
	if err != nil {
		return "", err
	}

	switch wireMode {
	case string(RoutingModeAuto):
		return RoutingModeAuto, nil
	case string(RoutingModeLocal):
		return RoutingModeLocal, nil
	case string(RoutingModeRemote):
		return RoutingModeRemote, nil
	case routingWireModeSecondary:
		return RoutingModeBackup, nil
	default:
		return "", fmt.Errorf(
			"cPanel returned invalid %s mode %q",
			name,
			wireMode,
		)
	}
}

func routingModeToWire(mode RoutingMode) (string, error) {
	switch mode {
	case RoutingModeAuto, RoutingModeLocal, RoutingModeRemote:
		return string(mode), nil
	case RoutingModeBackup:
		return routingWireModeSecondary, nil
	default:
		return "", fmt.Errorf(
			"email routing mode must be auto, backup, local, or remote, got %q",
			mode,
		)
	}
}

func parseRequiredRoutingString(
	name string,
	raw json.RawMessage,
) (string, error) {
	value, err := parseNullableRoutingString(name, raw)
	if err != nil {
		return "", err
	}
	if value == nil || *value == "" {
		return "", fmt.Errorf("cPanel returned an empty %s", name)
	}

	return *value, nil
}

func parseNullableRoutingString(
	name string,
	raw json.RawMessage,
) (*string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, nil
	}

	var decoded string
	if err := json.Unmarshal(value, &decoded); err != nil {
		return nil, fmt.Errorf("decode cPanel %s: %w", name, err)
	}
	if strings.TrimSpace(decoded) != decoded {
		return nil, fmt.Errorf(
			"cPanel returned %s %q with surrounding whitespace",
			name,
			decoded,
		)
	}

	return &decoded, nil
}

func parseRoutingFlag(
	name string,
	raw json.RawMessage,
) (bool, error) {
	value, err := parseBooleanFlagJSON(raw)
	if err != nil {
		return false, fmt.Errorf(
			"parse cPanel %s flag: %w",
			name,
			err,
		)
	}

	return value, nil
}

func parseRoutingInteger(
	name string,
	raw json.RawMessage,
	minimum int64,
) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("cPanel returned a missing %s", name)
	}

	text := string(value)
	if value[0] == '"' {
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return 0, fmt.Errorf("decode cPanel %s: %w", name, err)
		}
		if decoded == "" || strings.TrimSpace(decoded) != decoded {
			return 0, fmt.Errorf(
				"cPanel returned invalid %s %q",
				name,
				decoded,
			)
		}
		text = decoded
	}

	result, err := strconv.ParseInt(text, 10, 64)
	if err != nil || result < minimum {
		return 0, fmt.Errorf(
			"cPanel returned invalid %s %q",
			name,
			text,
		)
	}

	return result, nil
}

func validateRoutingDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("email routing domain must not be empty")
	}
	if strings.TrimSpace(domain) != domain {
		return fmt.Errorf(
			"email routing domain %q must not contain surrounding whitespace",
			domain,
		)
	}

	return nil
}

func validRoutingMode(mode RoutingMode) bool {
	switch mode {
	case RoutingModeAuto,
		RoutingModeBackup,
		RoutingModeLocal,
		RoutingModeRemote:
		return true
	default:
		return false
	}
}

func routingMXEntryError(
	domain string,
	index int,
	err error,
) error {
	return fmt.Errorf(
		"domain %q MX entry at index %d: %w",
		domain,
		index,
		err,
	)
}
