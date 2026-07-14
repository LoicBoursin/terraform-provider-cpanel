package mysql

import (
	"context"
	"fmt"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"terraform-provider-cpanel/internal/cpanel"
)

func (c *Client) ListRemoteHosts(ctx context.Context) ([]RemoteHost, error) {
	hostResponse := RemoteHostListResponse{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleMysqlFE,
		operationListRemoteHosts,
		map[string]string{},
		&hostResponse,
	); err != nil {
		return nil, err
	}

	noteResponse := RemoteHostNotesResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationGetRemoteHostNotes,
		map[string]string{},
		&noteResponse,
	); err != nil {
		return nil, err
	}

	remoteHosts := make([]RemoteHost, 0, len(hostResponse.CpanelResult.Data))
	seenHosts := make(map[string]struct{}, len(hostResponse.CpanelResult.Data))
	for _, apiHost := range hostResponse.CpanelResult.Data {
		host, err := NormalizeRemoteHost(apiHost.Host)
		if err != nil {
			return nil, fmt.Errorf(
				"normalize remote MySQL host %q: %w",
				apiHost.Host,
				err,
			)
		}
		if _, duplicate := seenHosts[host]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate remote MySQL host %q",
				host,
			)
		}
		seenHosts[host] = struct{}{}

		note := noteResponse.Data[apiHost.Host]
		if normalizedNote, ok := noteResponse.Data[host]; ok {
			note = normalizedNote
		}
		remoteHosts = append(remoteHosts, RemoteHost{
			Host: host,
			Note: note,
		})
	}

	slices.SortFunc(remoteHosts, func(left, right RemoteHost) int {
		return strings.Compare(left.Host, right.Host)
	})

	return remoteHosts, nil
}

func (c *Client) GetRemoteHost(
	ctx context.Context,
	host string,
) (*RemoteHost, error) {
	normalizedHost, err := NormalizeRemoteHost(host)
	if err != nil {
		return nil, err
	}

	remoteHosts, err := c.ListRemoteHosts(ctx)
	if err != nil {
		return nil, err
	}
	for _, remoteHost := range remoteHosts {
		if remoteHost.Host == normalizedHost {
			remoteHostCopy := remoteHost

			return &remoteHostCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) AddRemoteHost(ctx context.Context, host string) error {
	response := MutationResponse{}

	return c.executeMutation(
		ctx,
		operationAddRemoteHost,
		map[string]string{"host": host},
		&response,
	)
}

func (c *Client) SetRemoteHostNote(
	ctx context.Context,
	host string,
	note string,
) error {
	response := MutationResponse{}

	return c.executeMutation(
		ctx,
		operationAddRemoteHostNote,
		map[string]string{
			"host": host,
			"note": note,
		},
		&response,
	)
}

func (c *Client) DeleteRemoteHost(ctx context.Context, host string) error {
	response := MutationResponse{}

	return c.executeMutation(
		ctx,
		operationDeleteRemoteHost,
		map[string]string{"host": host},
		&response,
	)
}

func NormalizeRemoteHost(host string) (string, error) {
	if host == "" {
		return "", fmt.Errorf("remote MySQL host must not be empty")
	}
	if strings.TrimSpace(host) != host {
		return "", fmt.Errorf(
			"remote MySQL host must not have leading or trailing whitespace",
		)
	}
	for _, character := range host {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return "", fmt.Errorf(
				"remote MySQL host must not contain whitespace or control characters",
			)
		}
	}

	if strings.Contains(host, "%") {
		return normalizeRemoteHostWildcard(host)
	}
	if strings.Contains(host, "/") {
		prefix, err := netip.ParsePrefix(host)
		if err != nil || !prefix.Addr().Is4() {
			return "", fmt.Errorf(
				"remote MySQL host CIDR must be a valid IPv4 prefix",
			)
		}

		return prefix.Masked().String(), nil
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if !address.Is4() {
			return "", fmt.Errorf(
				"remote MySQL host IP address must use IPv4",
			)
		}

		return address.String(), nil
	}
	if strings.Contains(host, ":") {
		return "", fmt.Errorf(
			"remote MySQL host IP address must use IPv4",
		)
	}

	return normalizeRemoteHostname(host)
}

func normalizeRemoteHostWildcard(host string) (string, error) {
	parts := strings.Split(host, ".")
	if len(parts) != 4 {
		return "", fmt.Errorf(
			"remote MySQL wildcard host must contain four IPv4 octets",
		)
	}

	normalizedParts := make([]string, len(parts))
	for index, part := range parts {
		if part == "%" {
			normalizedParts[index] = part
			continue
		}
		octet, err := strconv.Atoi(part)
		if err != nil || octet < 0 || octet > 255 {
			return "", fmt.Errorf(
				"remote MySQL wildcard host octets must be 0-255 or %%",
			)
		}
		normalizedParts[index] = strconv.Itoa(octet)
	}

	return strings.Join(normalizedParts, "."), nil
}

func normalizeRemoteHostname(host string) (string, error) {
	host = strings.ToLower(host)
	if len(host) > 253 {
		return "", fmt.Errorf(
			"remote MySQL hostname must not exceed 253 bytes",
		)
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", fmt.Errorf(
			"remote MySQL hostname must not start or end with a dot",
		)
	}

	allNumeric := true
	for _, character := range host {
		if character != '.' && !unicode.IsDigit(character) {
			allNumeric = false
			break
		}
	}
	if allNumeric {
		return "", fmt.Errorf(
			"remote MySQL host is not a valid IPv4 address",
		)
	}

	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 {
			return "", fmt.Errorf(
				"remote MySQL hostname labels must contain 1-63 bytes",
			)
		}
		for index, character := range label {
			isLetter := character >= 'a' && character <= 'z'
			isDigit := character >= '0' && character <= '9'
			if !isLetter && !isDigit &&
				(character != '-' || index == 0 || index == len(label)-1) {
				return "", fmt.Errorf(
					"remote MySQL hostname contains an invalid label",
				)
			}
		}
	}

	return host, nil
}
