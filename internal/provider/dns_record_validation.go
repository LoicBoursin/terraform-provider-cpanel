package provider

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var dnsRecordLabelPattern = regexp.MustCompile(
	`^(?:\*|[A-Za-z0-9_](?:[A-Za-z0-9_-]*[A-Za-z0-9_])?)$`,
)

var supportedDNSRecordTypes = []string{
	"A",
	"AAAA",
	"CAA",
	"CNAME",
	"MX",
	"SRV",
	"TXT",
}

func validateDNSRecord(zone, name, recordType string, ttl int64, data []string) error {
	if err := validateDomainName(zone); err != nil {
		return fmt.Errorf("invalid DNS zone: %w", err)
	}
	if err := validateDNSRecordName(zone, name); err != nil {
		return err
	}
	if !containsString(supportedDNSRecordTypes, recordType) {
		return fmt.Errorf(
			"unsupported DNS record type %q; expected one of %s",
			recordType,
			strings.Join(supportedDNSRecordTypes, ", "),
		)
	}
	if ttl < 1 || ttl > 2147483647 {
		return errors.New("record TTL must be between 1 and 2147483647 seconds")
	}
	if len(data) == 0 {
		return errors.New("record data must contain at least one value")
	}

	for index, value := range data {
		if value == "" {
			return fmt.Errorf("record data item %d must not be empty", index)
		}
	}

	switch recordType {
	case "A":
		if err := requireDNSDataLength(recordType, data, 1); err != nil {
			return err
		}
		ip := net.ParseIP(data[0])
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("record data %q is not a valid IPv4 address", data[0])
		}
	case "AAAA":
		if err := requireDNSDataLength(recordType, data, 1); err != nil {
			return err
		}
		ip := net.ParseIP(data[0])
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("record data %q is not a valid IPv6 address", data[0])
		}
	case "CAA":
		if err := requireDNSDataLength(recordType, data, 3); err != nil {
			return err
		}
		if err := validateBoundedInteger(data[0], 0, 255, "CAA flag"); err != nil {
			return err
		}
		if !dnsRecordLabelPattern.MatchString(data[1]) {
			return fmt.Errorf("record CAA tag %q is invalid", data[1])
		}
	case "CNAME":
		if err := requireDNSDataLength(recordType, data, 1); err != nil {
			return err
		}
		if err := validateDNSTarget(data[0], false); err != nil {
			return fmt.Errorf("%s record target: %w", recordType, err)
		}
	case "MX":
		if err := requireDNSDataLength(recordType, data, 2); err != nil {
			return err
		}
		if err := validateBoundedInteger(data[0], 0, 65535, "MX preference"); err != nil {
			return err
		}
		if err := validateDNSTarget(data[1], false); err != nil {
			return fmt.Errorf("record MX exchange: %w", err)
		}
	case "SRV":
		if err := requireDNSDataLength(recordType, data, 4); err != nil {
			return err
		}
		for index, label := range []string{"priority", "weight", "port"} {
			if err := validateBoundedInteger(
				data[index],
				0,
				65535,
				"SRV "+label,
			); err != nil {
				return err
			}
		}
		if err := validateDNSTarget(data[3], true); err != nil {
			return fmt.Errorf("record SRV target: %w", err)
		}
	case "TXT":
		for index, value := range data {
			if len([]byte(value)) > 255 {
				return fmt.Errorf(
					"TXT record data item %d exceeds the 255-byte DNS string limit",
					index,
				)
			}
		}
	}

	return nil
}

func validateDNSRecordName(zone, name string) error {
	if name == "@" {
		return nil
	}
	if name == "" {
		return errors.New("record name must not be empty")
	}
	if len(name) > 253 {
		return errors.New("record name must not exceed 253 characters")
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
		return errors.New("record name must be relative to the zone without surrounding dots")
	}

	normalizedZone := strings.TrimSuffix(zone, ".")
	if name == normalizedZone || strings.HasSuffix(name, "."+normalizedZone) {
		return errors.New(
			"record name must be relative to the zone; use @ for the zone apex",
		)
	}

	for _, label := range strings.Split(name, ".") {
		if len(label) > 63 {
			return fmt.Errorf("record label %q exceeds 63 characters", label)
		}
		if !dnsRecordLabelPattern.MatchString(label) {
			return fmt.Errorf("record label %q is invalid", label)
		}
	}

	return nil
}

func validateDNSTarget(target string, allowRoot bool) error {
	if allowRoot && target == "." {
		return nil
	}

	target = strings.TrimSuffix(target, ".")
	if target == "" {
		return errors.New("target must not be empty")
	}
	if err := validateDomainName(target); err != nil {
		return err
	}

	return nil
}

func requireDNSDataLength(recordType string, data []string, expected int) error {
	if len(data) != expected {
		return fmt.Errorf(
			"%s record data must contain exactly %d value(s), got %d",
			recordType,
			expected,
			len(data),
		)
	}

	return nil
}

func validateBoundedInteger(value string, minimum, maximum int64, label string) error {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < minimum || parsed > maximum {
		return fmt.Errorf(
			"%s must be an integer between %d and %d",
			label,
			minimum,
			maximum,
		)
	}

	return nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}

	return false
}
