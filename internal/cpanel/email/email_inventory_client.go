package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	emailInventoryDomainLabelPattern = regexp.MustCompile(`^[a-z0-9-]+$`)
	emailInventoryLocalPartPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type emailInventoryResponse struct {
	Data     json.RawMessage `json:"data"`
	Warnings []string        `json:"warnings"`
}

type apiAccountInventory struct {
	Address json.RawMessage `json:"email"`
}

type apiMailDomainInventory struct {
	Domain json.RawMessage `json:"domain"`
}

type apiDomainForwarderInventory struct {
	Domain      json.RawMessage `json:"dest"`
	Destination json.RawMessage `json:"forward"`
}

type apiMailingListAddressInventory struct {
	Address json.RawMessage `json:"list"`
}

type apiAutoResponderAddressInventory struct {
	Address json.RawMessage `json:"email"`
}

func (c *Client) ListAccountAddresses(
	ctx context.Context,
) ([]string, error) {
	response := emailInventoryResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListAddresses,
		map[string]string{
			"skip_main": "1",
		},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectEmailInventoryWarnings(
		"email account inventory",
		response.Warnings,
	); err != nil {
		return nil, err
	}

	rows, err := decodeEmailInventoryRows[apiAccountInventory](
		response.Data,
		"email account inventory",
	)
	if err != nil {
		return nil, err
	}

	return normalizeUniqueEmailInventoryStrings(
		rows,
		"email account address",
		func(row apiAccountInventory) (string, error) {
			return normalizeEmailInventoryAddressRaw(row.Address)
		},
	)
}

func (c *Client) listMailDomainsStrict(
	ctx context.Context,
) ([]string, error) {
	response := emailInventoryResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListMailDomains,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectEmailInventoryWarnings(
		"mail domain inventory",
		response.Warnings,
	); err != nil {
		return nil, err
	}

	rows, err := decodeEmailInventoryRows[apiMailDomainInventory](
		response.Data,
		"mail domain inventory",
	)
	if err != nil {
		return nil, err
	}

	return normalizeUniqueEmailInventoryStringsInOrder(
		rows,
		"mail domain",
		func(row apiMailDomainInventory) (string, error) {
			return normalizeEmailInventoryDomainRaw(row.Domain)
		},
	)
}

func (c *Client) ListMailDomainInventory(
	ctx context.Context,
) ([]string, error) {
	domains, err := c.listMailDomainsStrict(ctx)
	if err != nil {
		return nil, err
	}
	sort.Strings(domains)

	return domains, nil
}

func (c *Client) ListRoutingDefinitions(
	ctx context.Context,
) ([]RoutingDefinition, error) {
	routings, err := c.ListRoutings(ctx)
	if err != nil {
		return nil, err
	}

	definitions := make([]RoutingDefinition, 0, len(routings))
	seenDomains := make(map[string]struct{}, len(routings))
	for index, routing := range routings {
		domain, err := normalizeEmailInventoryDomain(routing.Domain)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid email routing at index %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seenDomains[domain]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate email routing domain %q",
				domain,
			)
		}
		seenDomains[domain] = struct{}{}
		definitions = append(definitions, RoutingDefinition{
			Domain: domain,
			Mode:   routing.Mode,
		})
	}
	sort.Slice(definitions, func(left, right int) bool {
		return definitions[left].Domain < definitions[right].Domain
	})

	return definitions, nil
}

func (c *Client) listDomainForwardersStrict(
	ctx context.Context,
) ([]DomainForwarder, error) {
	response := emailInventoryResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListDomainFwds,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectEmailInventoryWarnings(
		"email domain forwarder inventory",
		response.Warnings,
	); err != nil {
		return nil, err
	}

	rows, err := decodeEmailInventoryRows[apiDomainForwarderInventory](
		response.Data,
		"email domain forwarder inventory",
	)
	if err != nil {
		return nil, err
	}

	forwarders := make([]DomainForwarder, 0, len(rows))
	seenDomains := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		domain, err := normalizeEmailInventoryDomainRaw(row.Domain)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid email domain forwarder at index %d: %w",
				index,
				err,
			)
		}
		destination, err := normalizeEmailInventoryDomainRaw(
			row.Destination,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid destination for email domain forwarder %q: %w",
				domain,
				err,
			)
		}
		if _, duplicate := seenDomains[domain]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate email domain forwarder for %q",
				domain,
			)
		}
		seenDomains[domain] = struct{}{}
		forwarders = append(forwarders, DomainForwarder{
			Domain:      domain,
			Destination: destination,
		})
	}
	sort.Slice(forwarders, func(left, right int) bool {
		if forwarders[left].Domain != forwarders[right].Domain {
			return forwarders[left].Domain < forwarders[right].Domain
		}

		return forwarders[left].Destination <
			forwarders[right].Destination
	})

	return forwarders, nil
}

func (c *Client) ListMailingListAddresses(
	ctx context.Context,
) ([]string, error) {
	response := emailInventoryResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListMailingLists,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if err := rejectEmailInventoryWarnings(
		"mailing list inventory",
		response.Warnings,
	); err != nil {
		return nil, err
	}

	rows, err := decodeEmailInventoryRows[apiMailingListAddressInventory](
		response.Data,
		"mailing list inventory",
	)
	if err != nil {
		return nil, err
	}

	return normalizeUniqueEmailInventoryStrings(
		rows,
		"mailing list address",
		func(row apiMailingListAddressInventory) (string, error) {
			address, err := parseRequiredMailingListString(row.Address)
			if err != nil {
				return "", err
			}
			if err := validateMailingListIdentifier(
				"address",
				address,
			); err != nil {
				return "", err
			}

			return normalizeEmailInventoryAddress(address)
		},
	)
}

func (c *Client) ListAutoResponderAddresses(
	ctx context.Context,
) ([]string, error) {
	domains, err := c.ListMailDomainInventory(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"list mail domains for autoresponder inventory: %w",
			err,
		)
	}

	addresses := make([]string, 0)
	seenAddresses := make(map[string]struct{})
	for _, domain := range domains {
		response := emailInventoryResponse{}
		if err := c.executeReadOperation(
			ctx,
			operationListAutoResp,
			map[string]string{"domain": domain},
			&response,
		); err != nil {
			return nil, fmt.Errorf(
				"list autoresponders for mail domain %q: %w",
				domain,
				err,
			)
		}
		if err := rejectEmailInventoryWarnings(
			fmt.Sprintf(
				"autoresponder inventory for mail domain %q",
				domain,
			),
			response.Warnings,
		); err != nil {
			return nil, err
		}

		rows, err := decodeEmailInventoryRows[apiAutoResponderAddressInventory](
			response.Data,
			fmt.Sprintf("autoresponder inventory for %q", domain),
		)
		if err != nil {
			return nil, err
		}
		for index, row := range rows {
			address, err := normalizeEmailInventoryAddressRaw(
				row.Address,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"invalid autoresponder address for mail domain %q at index %d: %w",
					domain,
					index,
					err,
				)
			}
			_, addressDomain, _ := strings.Cut(address, "@")
			if addressDomain != domain {
				return nil, fmt.Errorf(
					"autoresponder address %q belongs to domain %q, expected %q",
					address,
					addressDomain,
					domain,
				)
			}
			if _, duplicate := seenAddresses[address]; duplicate {
				return nil, fmt.Errorf(
					"cPanel returned duplicate autoresponder address %q",
					address,
				)
			}
			seenAddresses[address] = struct{}{}
			addresses = append(addresses, address)
		}
	}
	sort.Strings(addresses)

	return addresses, nil
}

func decodeEmailInventoryRows[T any](
	raw json.RawMessage,
	label string,
) ([]T, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s data is missing or null", label)
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("%s data must be an array", label)
	}

	var rawRows []json.RawMessage
	if err := json.Unmarshal(value, &rawRows); err != nil {
		return nil, fmt.Errorf("decode %s data: %w", label, err)
	}

	rows := make([]T, 0, len(rawRows))
	for index, rawRow := range rawRows {
		rowValue := bytes.TrimSpace(rawRow)
		if len(rowValue) == 0 ||
			bytes.Equal(rowValue, []byte("null")) ||
			rowValue[0] != '{' {
			return nil, fmt.Errorf(
				"%s entry at index %d must be an object",
				label,
				index,
			)
		}

		var row T
		if err := json.Unmarshal(rowValue, &row); err != nil {
			return nil, fmt.Errorf(
				"decode %s entry at index %d: %w",
				label,
				index,
				err,
			)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

func normalizeUniqueEmailInventoryStrings[T any](
	rows []T,
	label string,
	normalize func(T) (string, error),
) ([]string, error) {
	values, err := normalizeUniqueEmailInventoryStringsInOrder(
		rows,
		label,
		normalize,
	)
	if err != nil {
		return nil, err
	}
	sort.Strings(values)

	return values, nil
}

func normalizeUniqueEmailInventoryStringsInOrder[T any](
	rows []T,
	label string,
	normalize func(T) (string, error),
) ([]string, error) {
	values := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		value, err := normalize(row)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid %s at index %d: %w",
				label,
				index,
				err,
			)
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate %s %q",
				label,
				value,
			)
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}

	return values, nil
}

func rejectEmailInventoryWarnings(
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

func normalizeEmailInventoryAddressRaw(
	raw json.RawMessage,
) (string, error) {
	address, err := parseRequiredMailingListString(raw)
	if err != nil {
		return "", err
	}

	return normalizeEmailInventoryAddress(address)
}

func normalizeEmailInventoryAddress(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf(
			"email address must be a non-empty trimmed string",
		)
	}
	if strings.Count(value, "@") != 1 {
		return "", fmt.Errorf("email address must contain exactly one @")
	}

	localPart, domain, _ := strings.Cut(value, "@")
	if localPart == "" || len(localPart) > 64 ||
		!emailInventoryLocalPartPattern.MatchString(localPart) ||
		strings.HasPrefix(localPart, ".") ||
		strings.HasSuffix(localPart, ".") ||
		strings.Contains(localPart, "..") {
		return "", fmt.Errorf(
			"email address %q contains an invalid local part",
			value,
		)
	}
	normalizedDomain, err := normalizeEmailInventoryDomain(domain)
	if err != nil {
		return "", fmt.Errorf(
			"email address %q: %w",
			value,
			err,
		)
	}

	normalized := localPart + "@" + normalizedDomain
	if len(normalized) > 254 {
		return "", fmt.Errorf(
			"email address %q exceeds 254 ASCII characters",
			value,
		)
	}

	return normalized, nil
}

func normalizeEmailInventoryDomainRaw(
	raw json.RawMessage,
) (string, error) {
	domain, err := parseRequiredMailingListString(raw)
	if err != nil {
		return "", err
	}

	return normalizeEmailInventoryDomain(domain)
}

func normalizeEmailInventoryDomain(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf(
			"domain name must be a non-empty trimmed string",
		)
	}

	normalized := strings.ToLower(strings.TrimSuffix(value, "."))
	if len(normalized) < 3 || len(normalized) > 253 {
		return "", fmt.Errorf(
			"domain name %q must contain between 3 and 253 ASCII characters",
			value,
		)
	}
	labels := strings.Split(normalized, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf(
			"domain name %q must contain at least one dot",
			value,
		)
	}
	for _, label := range labels {
		if label == "" ||
			len(label) > 63 ||
			!emailInventoryDomainLabelPattern.MatchString(label) ||
			strings.HasPrefix(label, "-") ||
			strings.HasSuffix(label, "-") {
			return "", fmt.Errorf(
				"domain name %q contains an invalid label",
				value,
			)
		}
	}

	return normalized, nil
}
