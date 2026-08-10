package ipblock

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
)

func NormalizeAddress(value string) (string, error) {
	normalized, _, _, err := normalizeAddressRange(value)

	return normalized, err
}

func normalizeAddressRange(
	value string,
) (string, netip.Addr, netip.Addr, error) {
	if value == "" {
		return "", netip.Addr{}, netip.Addr{},
			errors.New("blocked address must not be empty")
	}
	if strings.TrimSpace(value) != value {
		return "", netip.Addr{}, netip.Addr{},
			errors.New("blocked address must not contain surrounding whitespace")
	}

	if strings.Contains(value, "-") {
		startValue, endValue, found := strings.Cut(value, "-")
		if !found || startValue == "" || endValue == "" ||
			strings.Contains(endValue, "-") {
			return "", netip.Addr{}, netip.Addr{}, errors.New(
				"blocked address range must use start-end form",
			)
		}

		start, err := netip.ParseAddr(startValue)
		if err != nil {
			return "", netip.Addr{}, netip.Addr{},
				fmt.Errorf("parse blocked range start: %w", err)
		}
		end, err := netip.ParseAddr(endValue)
		if err != nil {
			return "", netip.Addr{}, netip.Addr{},
				fmt.Errorf("parse blocked range end: %w", err)
		}
		if start.BitLen() != end.BitLen() {
			return "", netip.Addr{}, netip.Addr{}, errors.New(
				"blocked address range endpoints must use the same IP version",
			)
		}
		if end.Less(start) {
			return "", netip.Addr{}, netip.Addr{}, errors.New(
				"blocked address range end must not precede its start",
			)
		}

		return start.String() + "-" + end.String(), start, end, nil
	}

	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return "", netip.Addr{}, netip.Addr{},
				fmt.Errorf("parse blocked CIDR prefix: %w", err)
		}
		prefix = prefix.Masked()

		return prefix.String(), prefix.Addr(), prefixLastAddress(prefix), nil
	}

	address, err := netip.ParseAddr(value)
	if err != nil {
		return "", netip.Addr{}, netip.Addr{},
			fmt.Errorf("parse blocked IP address: %w", err)
	}

	return address.String(), address, address, nil
}

func prefixLastAddress(prefix netip.Prefix) netip.Addr {
	address := prefix.Addr()
	bytes := address.AsSlice()
	for bit := prefix.Bits(); bit < address.BitLen(); bit++ {
		byteIndex := bit / 8
		bitIndex := uint(7 - bit%8)
		bytes[byteIndex] |= 1 << bitIndex
	}

	if address.Is4() {
		return netip.AddrFrom4([4]byte(bytes))
	}

	return netip.AddrFrom16([16]byte(bytes))
}
