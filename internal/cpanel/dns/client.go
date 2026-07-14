package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	mutationMu sync.Mutex
}

type recordMutation struct {
	Name       string   `json:"dname"`
	TTL        int64    `json:"ttl"`
	RecordType string   `json:"record_type"`
	Data       []string `json:"data"`
}

type recordEditMutation struct {
	LineIndex int64 `json:"line_index"`
	recordMutation
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) ParseZone(ctx context.Context, zone string) (*Zone, error) {
	response := ParseZoneResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleDNS,
		operationParseZone,
		map[string]string{"zone": zone},
		&response,
	); err != nil {
		return nil, err
	}

	parsedZone, err := newZone(zone, response.Data)
	if err != nil {
		return nil, fmt.Errorf("decode cPanel DNS zone %q: %w", zone, err)
	}

	return parsedZone, nil
}

func (c *Client) AddRecord(
	ctx context.Context,
	zone string,
	record Record,
) error {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	currentZone, err := c.ParseZone(ctx, zone)
	if err != nil {
		return err
	}
	if len(currentZone.ExactRecords(record)) > 0 {
		return ErrRecordAlreadyExists
	}

	payload, err := json.Marshal(recordMutation{
		Name:       apiRecordName(zone, record.Name),
		TTL:        record.TTL,
		RecordType: record.Type,
		Data:       record.Data,
	})
	if err != nil {
		return fmt.Errorf("encode DNS record addition: %w", err)
	}

	mutationErr := c.massEditZone(ctx, zone, currentZone.Serial, map[string]string{
		"add": string(payload),
	})
	if mutationErr == nil || mutationErrorIsDeterministic(mutationErr) {
		return mutationErr
	}

	reconciledZone, readErr := c.ParseZone(ctx, zone)
	if readErr != nil {
		return errors.Join(
			mutationErr,
			fmt.Errorf("read DNS zone after ambiguous record addition: %w", readErr),
		)
	}

	switch len(reconciledZone.ExactRecords(record)) {
	case 0:
		return errors.Join(
			mutationErr,
			fmt.Errorf("the requested DNS record was not found after the ambiguous addition"),
		)
	case 1:
		return nil
	default:
		return errors.Join(
			mutationErr,
			ErrRecordAmbiguous,
		)
	}
}

func (c *Client) UpdateRecord(
	ctx context.Context,
	zone string,
	current Record,
	desired Record,
) (int64, error) {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	currentZone, err := c.ParseZone(ctx, zone)
	if err != nil {
		return 0, err
	}
	resolved, err := currentZone.LocateRecord(current)
	if err != nil {
		return 0, err
	}
	desired.LineIndex = resolved.LineIndex

	payload, err := json.Marshal(recordEditMutation{
		LineIndex: desired.LineIndex,
		recordMutation: recordMutation{
			Name:       apiRecordName(zone, desired.Name),
			TTL:        desired.TTL,
			RecordType: desired.Type,
			Data:       desired.Data,
		},
	})
	if err != nil {
		return 0, fmt.Errorf("encode DNS record edit: %w", err)
	}

	mutationErr := c.massEditZone(ctx, zone, currentZone.Serial, map[string]string{
		"edit": string(payload),
	})
	if mutationErr == nil || mutationErrorIsDeterministic(mutationErr) {
		return desired.LineIndex, mutationErr
	}

	reconciledZone, readErr := c.ParseZone(ctx, zone)
	if readErr != nil {
		return 0, errors.Join(
			mutationErr,
			fmt.Errorf("read DNS zone after ambiguous record update: %w", readErr),
		)
	}

	desiredMatches := reconciledZone.ExactRecords(desired)
	if len(desiredMatches) > 1 {
		return 0, errors.Join(mutationErr, ErrRecordAmbiguous)
	}
	currentMatches := reconciledZone.ExactRecords(current)
	if len(desiredMatches) == 1 && len(currentMatches) == 0 {
		return desiredMatches[0].LineIndex, nil
	}
	if len(desiredMatches) == 1 {
		return 0, errors.Join(
			mutationErr,
			errors.New("both the previous and requested DNS records exist after the ambiguous update"),
		)
	}
	if len(currentMatches) == 1 {
		return 0, errors.Join(
			mutationErr,
			errors.New("the previous DNS record still exists after the ambiguous update"),
		)
	}

	return 0, errors.Join(
		mutationErr,
		errors.New("the requested DNS record was not found after the ambiguous update"),
	)
}

func (c *Client) DeleteRecord(
	ctx context.Context,
	zone string,
	record Record,
) error {
	c.mutationMu.Lock()
	defer c.mutationMu.Unlock()

	currentZone, err := c.ParseZone(ctx, zone)
	if err != nil {
		return err
	}
	resolved, err := currentZone.LocateRecord(record)
	if err != nil {
		return err
	}

	return c.massEditZone(ctx, zone, currentZone.Serial, map[string]string{
		"remove": strconv.FormatInt(resolved.LineIndex, 10),
	})
}

func (c *Client) massEditZone(
	ctx context.Context,
	zone string,
	serial int64,
	mutation map[string]string,
) error {
	parameters := map[string]string{
		"zone":   zone,
		"serial": strconv.FormatInt(serial, 10),
	}
	for key, value := range mutation {
		parameters[key] = value
	}

	response := MutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleDNS,
		operationEditZone,
		parameters,
		&response,
	)
}

func apiRecordName(zone, name string) string {
	if name == "@" {
		return zone + "."
	}

	return name
}

func mutationErrorIsDeterministic(err error) bool {
	var apiError *cpanel.APIError
	if errors.As(err, &apiError) {
		return true
	}

	var httpError *cpanel.HTTPError

	return errors.As(err, &httpError) &&
		httpError.StatusCode >= 400 &&
		httpError.StatusCode < 500
}
