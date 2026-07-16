package domain

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) ListDomains(
	ctx context.Context,
) ([]InventoryDomain, error) {
	domainList, err := c.getDomainList(ctx)
	if err != nil {
		return nil, err
	}

	return normalizeDomainInventory(domainList)
}

func (c *Client) getDomainList(
	ctx context.Context,
) (DomainList, error) {
	response := DomainListResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDomainInfo,
		"list_domains",
		map[string]string{},
		&response,
	); err != nil {
		return DomainList{}, err
	}

	return response.Data, nil
}

func normalizeDomainInventory(
	domainList DomainList,
) ([]InventoryDomain, error) {
	result := make(
		[]InventoryDomain,
		0,
		1+
			len(domainList.AddonDomains)+
			len(domainList.SubDomains)+
			len(domainList.ParkedDomains),
	)
	seen := make(map[string]InventoryDomainType, cap(result))
	appendDomain := func(
		rawName string,
		domainType InventoryDomainType,
	) error {
		name, err := normalizeInventoryDomainName(rawName)
		if err != nil {
			return fmt.Errorf(
				"decode cPanel %s domain: %w",
				domainType,
				err,
			)
		}
		if existingType, duplicate := seen[name]; duplicate {
			return fmt.Errorf(
				"cPanel domain %q appears as both %s and %s",
				name,
				existingType,
				domainType,
			)
		}
		seen[name] = domainType
		result = append(result, InventoryDomain{
			Name: name,
			Type: domainType,
		})

		return nil
	}

	if err := appendDomain(
		domainList.MainDomain,
		InventoryDomainTypeMain,
	); err != nil {
		return nil, err
	}
	for _, group := range []struct {
		names      []string
		domainType InventoryDomainType
	}{
		{
			names:      domainList.AddonDomains,
			domainType: InventoryDomainTypeAddon,
		},
		{
			names:      domainList.SubDomains,
			domainType: InventoryDomainTypeSubdomain,
		},
		{
			names:      domainList.ParkedDomains,
			domainType: InventoryDomainTypeAlias,
		},
	} {
		for _, name := range group.names {
			if err := appendDomain(name, group.domainType); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Name == result[right].Name {
			return result[left].Type < result[right].Type
		}

		return result[left].Name < result[right].Name
	})

	return result, nil
}

func normalizeInventoryDomainName(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return "", fmt.Errorf(
			"domain name must be a non-empty trimmed string",
		)
	}
	normalized := strings.ToLower(strings.TrimSuffix(value, "."))
	if normalized == "" ||
		strings.ContainsAny(normalized, " \t\r\n") {
		return "", fmt.Errorf("domain name %q is invalid", value)
	}

	return normalized, nil
}
