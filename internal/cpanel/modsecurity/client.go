package modsecurity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) HasInstalled(ctx context.Context) (bool, error) {
	response := InstalledResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleModSecurity,
		operationHasInstalled,
		map[string]string{},
		&response,
	); err != nil {
		return false, err
	}

	switch response.Data.Installed {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf(
			"cPanel returned invalid ModSecurity installation status %d",
			response.Data.Installed,
		)
	}
}

func (c *Client) List(ctx context.Context) ([]Domain, error) {
	response := ListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleModSecurity,
		operationListDomains,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return normalizeAPIDomains(response.Data)
}

func (c *Client) Get(ctx context.Context, domain string) (*Domain, error) {
	normalizedDomain, err := normalizeDomain(domain)
	if err != nil {
		return nil, err
	}

	domains, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(domains), func(index int) bool {
		return domains[index].Domain >= normalizedDomain
	})
	if index == len(domains) || domains[index].Domain != normalizedDomain {
		return nil, nil
	}

	return &domains[index], nil
}

func (c *Client) SetEnabled(
	ctx context.Context,
	domains []string,
	enabled bool,
) ([]Domain, error) {
	normalizedDomains, err := normalizeDomains(domains)
	if err != nil {
		return nil, err
	}

	operation := operationDisableDomains
	if enabled {
		operation = operationEnableDomains
	}

	response := MutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleModSecurity,
		operation,
		map[string]string{"domains": strings.Join(normalizedDomains, ",")},
		&response,
	); err != nil {
		return nil, err
	}

	changedDomains, err := normalizeMutationData(response.Data, enabled)
	if err != nil {
		return nil, err
	}
	for _, requestedDomain := range normalizedDomains {
		found := false
		for _, changedDomain := range changedDomains {
			if changedDomain.Domain == requestedDomain {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf(
				"ModSecurity mutation did not return requested domain %q",
				requestedDomain,
			)
		}
	}

	return changedDomains, nil
}

func normalizeMutationData(data json.RawMessage, enabled bool) ([]Domain, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, fmt.Errorf("ModSecurity mutation returned no domain data")
	}
	if bytes.Equal(data, []byte("false")) {
		return nil, fmt.Errorf("ModSecurity mutation returned false")
	}

	var apiDomains []apiDomain
	if err := json.Unmarshal(data, &apiDomains); err != nil {
		return nil, fmt.Errorf("decode ModSecurity mutation data: %w", err)
	}
	if len(apiDomains) == 0 {
		return nil, fmt.Errorf("ModSecurity mutation returned no domains")
	}
	for _, apiDomain := range apiDomains {
		if apiDomain.Exception != "" {
			return nil, fmt.Errorf(
				"ModSecurity mutation failed for domain %q: %s",
				apiDomain.Domain,
				apiDomain.Exception,
			)
		}
	}

	domains, err := normalizeAPIDomains(apiDomains)
	if err != nil {
		return nil, err
	}
	for _, domain := range domains {
		if domain.Enabled != enabled {
			return nil, fmt.Errorf(
				"ModSecurity mutation returned enabled=%t for domain %q; expected %t",
				domain.Enabled,
				domain.Domain,
				enabled,
			)
		}
	}

	return domains, nil
}

func normalizeAPIDomains(apiDomains []apiDomain) ([]Domain, error) {
	domains := make([]Domain, 0, len(apiDomains))
	seen := make(map[string]struct{}, len(apiDomains))
	for _, apiDomain := range apiDomains {
		domain, err := normalizeAPIDomain(apiDomain)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[domain.Domain]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate ModSecurity domain %q",
				domain.Domain,
			)
		}
		seen[domain.Domain] = struct{}{}
		domains = append(domains, domain)
	}
	sort.Slice(domains, func(left, right int) bool {
		return domains[left].Domain < domains[right].Domain
	})

	return domains, nil
}

func normalizeAPIDomain(apiDomain apiDomain) (Domain, error) {
	domain, err := normalizeDomain(apiDomain.Domain)
	if err != nil {
		return Domain{}, fmt.Errorf("invalid ModSecurity domain: %w", err)
	}

	enabled := false
	switch apiDomain.Enabled {
	case 0:
	case 1:
		enabled = true
	default:
		return Domain{}, fmt.Errorf(
			"domain %q returned invalid ModSecurity status %d",
			domain,
			apiDomain.Enabled,
		)
	}

	if apiDomain.Type != DomainTypeMain && apiDomain.Type != DomainTypeSub {
		return Domain{}, fmt.Errorf(
			"domain %q returned unsupported ModSecurity domain type %q",
			domain,
			apiDomain.Type,
		)
	}

	dependencies, err := normalizeDomainsAllowEmpty(apiDomain.Dependencies)
	if err != nil {
		return Domain{}, fmt.Errorf(
			"domain %q returned invalid ModSecurity dependencies: %w",
			domain,
			err,
		)
	}

	return Domain{
		Domain:       domain,
		Enabled:      enabled,
		Dependencies: dependencies,
		SearchHint:   strings.TrimSpace(apiDomain.SearchHint),
		Type:         apiDomain.Type,
	}, nil
}

func normalizeDomains(domains []string) ([]string, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("at least one ModSecurity domain is required")
	}

	return normalizeDomainsAllowEmpty(domains)
}

func normalizeDomainsAllowEmpty(domains []string) ([]string, error) {
	normalized := make([]string, 0, len(domains))
	seen := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		value, err := normalizeDomain(domain)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("duplicate domain %q", value)
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)

	return normalized, nil
}

func normalizeDomain(domain string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(domain))
	if normalized == "" {
		return "", fmt.Errorf("domain must not be empty")
	}
	if normalized != domain {
		return "", fmt.Errorf("domain %q must be normalized and lowercase", domain)
	}

	return normalized, nil
}
