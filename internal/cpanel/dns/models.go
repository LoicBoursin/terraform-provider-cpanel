package dns

import (
	"encoding/base64"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

type ParseZoneResponse struct {
	cpanel.UAPIDataSourceModel
	Data []ParsedZoneEntry `json:"data"`
}

type ParsedZoneEntry struct {
	LineIndex  int64    `json:"line_index"`
	Type       string   `json:"type"`
	TextBase64 string   `json:"text_b64"`
	NameBase64 string   `json:"dname_b64"`
	TTL        int64    `json:"ttl"`
	RecordType string   `json:"record_type"`
	DataBase64 []string `json:"data_b64"`
}

type MutationResponse struct {
	cpanel.UAPIDataSourceModel
	Data struct {
		NewSerial any `json:"new_serial"`
	} `json:"data"`
}

type Record struct {
	LineIndex int64
	Name      string
	Type      string
	TTL       int64
	Data      []string
}

type Zone struct {
	Name    string
	Serial  int64
	Records []Record
}

var (
	ErrRecordAlreadyExists = errors.New("an identical DNS record already exists")
	ErrRecordNotFound      = errors.New("record was not found")
	ErrRecordAmbiguous     = errors.New("record identity is ambiguous")
)

func newZone(name string, entries []ParsedZoneEntry) (*Zone, error) {
	zone := &Zone{Name: strings.TrimSuffix(name, ".")}

	for _, entry := range entries {
		if entry.RecordType == "" {
			continue
		}

		record, err := decodeRecord(zone.Name, entry)
		if err != nil {
			return nil, err
		}
		zone.Records = append(zone.Records, record)

		if record.Type == "SOA" {
			if len(record.Data) < 3 {
				return nil, errors.New("cPanel DNS SOA record does not contain a serial")
			}
			serial, err := strconv.ParseInt(record.Data[2], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse cPanel DNS SOA serial %q: %w", record.Data[2], err)
			}
			zone.Serial = serial
		}
	}

	if zone.Serial <= 0 {
		return nil, errors.New("cPanel DNS zone does not contain a valid SOA serial")
	}

	return zone, nil
}

func decodeRecord(zone string, entry ParsedZoneEntry) (Record, error) {
	name, err := decodeBase64String(entry.NameBase64)
	if err != nil {
		return Record{}, fmt.Errorf(
			"decode DNS record name at line %d: %w",
			entry.LineIndex,
			err,
		)
	}

	data := make([]string, 0, len(entry.DataBase64))
	for index, encodedValue := range entry.DataBase64 {
		value, err := decodeBase64String(encodedValue)
		if err != nil {
			return Record{}, fmt.Errorf(
				"decode DNS record data item %d at line %d: %w",
				index,
				entry.LineIndex,
				err,
			)
		}
		data = append(data, value)
	}

	return Record{
		LineIndex: entry.LineIndex,
		Name:      normalizeRecordName(zone, name),
		Type:      strings.ToUpper(entry.RecordType),
		TTL:       entry.TTL,
		Data:      data,
	}, nil
}

func decodeBase64String(value string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func normalizeRecordName(zone, name string) string {
	zone = strings.TrimSuffix(zone, ".")
	name = strings.TrimSpace(name)
	withoutTrailingDot := strings.TrimSuffix(name, ".")

	if withoutTrailingDot == zone {
		return "@"
	}

	suffix := "." + zone
	if strings.HasSuffix(withoutTrailingDot, suffix) {
		return strings.TrimSuffix(withoutTrailingDot, suffix)
	}

	return withoutTrailingDot
}

func (z *Zone) RecordAt(lineIndex int64) *Record {
	for index := range z.Records {
		if z.Records[index].LineIndex == lineIndex {
			return &z.Records[index]
		}
	}

	return nil
}

func (z *Zone) RecordsByIdentity(name, recordType string) []Record {
	var matches []Record

	for _, record := range z.Records {
		if record.Name == name && record.Type == recordType {
			matches = append(matches, record)
		}
	}

	return matches
}

func (z *Zone) ExactRecords(expected Record) []Record {
	var matches []Record

	for _, record := range z.Records {
		if RecordsEqual(record, expected) {
			matches = append(matches, record)
		}
	}

	return matches
}

func (z *Zone) LocateRecord(expected Record) (*Record, error) {
	recordAtLine := z.RecordAt(expected.LineIndex)
	if recordAtLine != nil && RecordsEqual(*recordAtLine, expected) {
		return recordAtLine, nil
	}

	candidates := z.ExactRecords(expected)
	switch len(candidates) {
	case 0:
		return nil, ErrRecordNotFound
	case 1:
		return &candidates[0], nil
	}

	return nil, ErrRecordAmbiguous
}

func (z *Zone) LocateManagedRecord(expected Record) (*Record, error) {
	recordAtLine := z.RecordAt(expected.LineIndex)
	if recordAtLine != nil &&
		recordAtLine.Name == expected.Name &&
		recordAtLine.Type == expected.Type {
		return recordAtLine, nil
	}

	exactMatches := z.ExactRecords(expected)
	switch len(exactMatches) {
	case 1:
		return &exactMatches[0], nil
	case 0:
	default:
		return nil, ErrRecordAmbiguous
	}

	identityMatches := z.RecordsByIdentity(expected.Name, expected.Type)
	switch len(identityMatches) {
	case 0:
		return nil, ErrRecordNotFound
	case 1:
		return &identityMatches[0], nil
	default:
		return nil, ErrRecordAmbiguous
	}
}

func RecordsEqual(left, right Record) bool {
	return left.Name == right.Name &&
		left.Type == right.Type &&
		left.TTL == right.TTL &&
		slices.Equal(left.Data, right.Data)
}
