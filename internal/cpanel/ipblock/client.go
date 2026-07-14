package ipblock

import (
	"context"
	"net/http"
	"net/netip"
	"sort"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) AddAddress(ctx context.Context, address string) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleBlockIP,
		operationAddAddress,
		map[string]string{"ip": address},
		&response,
	)
}

func (c *Client) RemoveAddress(ctx context.Context, address string) error {
	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleBlockIP,
		operationRemoveAddress,
		map[string]string{"ip": address},
		&response,
	)
}

func (c *Client) ListAddresses(ctx context.Context) ([]BlockedAddress, error) {
	response := ListResponse{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDenyIP,
		operationListAddresses,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return response.CpanelResult.Data, nil
}

func (c *Client) GetAddress(
	ctx context.Context,
	address string,
) (*BlockedAddress, error) {
	expected, expectedStart, expectedEnd, err := normalizeAddressRange(address)
	if err != nil {
		return nil, err
	}

	addresses, err := c.ListAddresses(ctx)
	if err != nil {
		return nil, err
	}

	type blockedRange struct {
		start netip.Addr
		end   netip.Addr
	}

	ranges := make([]blockedRange, 0, len(addresses))
	for _, blockedAddress := range addresses {
		start, err := netip.ParseAddr(blockedAddress.Start)
		if err != nil || start.BitLen() != expectedStart.BitLen() {
			continue
		}
		end, err := netip.ParseAddr(blockedAddress.End)
		if err != nil || end.BitLen() != expectedEnd.BitLen() {
			continue
		}
		if start.Less(expectedStart) || expectedEnd.Less(end) {
			continue
		}

		ranges = append(ranges, blockedRange{start: start, end: end})
	}
	if len(ranges) == 0 {
		return nil, nil
	}

	sort.Slice(ranges, func(left, right int) bool {
		if ranges[left].start == ranges[right].start {
			return ranges[left].end.Less(ranges[right].end)
		}

		return ranges[left].start.Less(ranges[right].start)
	})

	if ranges[0].start != expectedStart {
		return nil, nil
	}

	coveredEnd := ranges[0].end
	for _, blockedRange := range ranges[1:] {
		nextAddress := coveredEnd.Next()
		if !nextAddress.IsValid() || nextAddress.Less(blockedRange.start) {
			break
		}
		if coveredEnd.Less(blockedRange.end) {
			coveredEnd = blockedRange.end
		}
	}
	if coveredEnd != expectedEnd {
		return nil, nil
	}

	return &BlockedAddress{
		Address: expected,
		Range:   expectedStart.String() + "-" + expectedEnd.String(),
		Start:   expectedStart.String(),
		End:     expectedEnd.String(),
	}, nil
}
