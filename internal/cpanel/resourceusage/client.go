package resourceusage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

var metricIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) Get(ctx context.Context) ([]Metric, error) {
	apiResponse := response{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleResourceUsage,
		operationGetUsages,
		map[string]string{},
		&apiResponse,
	); err != nil {
		return nil, err
	}
	if len(apiResponse.Data) == 0 {
		return nil, fmt.Errorf("cPanel returned an empty resource usage inventory")
	}

	metrics := make([]Metric, 0, len(apiResponse.Data))
	seenIDs := make(map[string]struct{}, len(apiResponse.Data))
	for _, apiMetric := range apiResponse.Data {
		metric, err := decodeMetric(apiMetric)
		if err != nil {
			return nil, err
		}
		if _, exists := seenIDs[metric.ID]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate resource usage metric %q",
				metric.ID,
			)
		}
		seenIDs[metric.ID] = struct{}{}
		metrics = append(metrics, metric)
	}

	slices.SortFunc(metrics, func(left, right Metric) int {
		return strings.Compare(left.ID, right.ID)
	})

	return metrics, nil
}

func decodeMetric(apiMetric apiMetric) (Metric, error) {
	id := strings.TrimSpace(apiMetric.ID)
	if !metricIDPattern.MatchString(id) {
		return Metric{}, fmt.Errorf(
			"cPanel returned invalid resource usage metric ID %q",
			apiMetric.ID,
		)
	}

	description := strings.TrimSpace(apiMetric.Description)
	if description == "" {
		return Metric{}, fmt.Errorf(
			"cPanel returned an empty description for resource usage metric %q",
			id,
		)
	}

	usage, err := decodeScalar(apiMetric.Usage, false)
	if err != nil {
		return Metric{}, fmt.Errorf(
			"decode cPanel resource usage metric %q usage: %w",
			id,
			err,
		)
	}
	maximum, err := decodeScalar(apiMetric.Maximum, true)
	if err != nil {
		return Metric{}, fmt.Errorf(
			"decode cPanel resource usage metric %q maximum: %w",
			id,
			err,
		)
	}
	formatter, err := decodeScalar(apiMetric.Formatter, true)
	if err != nil {
		return Metric{}, fmt.Errorf(
			"decode cPanel resource usage metric %q formatter: %w",
			id,
			err,
		)
	}
	metricError, err := decodeScalar(apiMetric.Error, true)
	if err != nil {
		return Metric{}, fmt.Errorf(
			"decode cPanel resource usage metric %q error: %w",
			id,
			err,
		)
	}

	return Metric{
		Description: description,
		Error:       metricError,
		Formatter:   formatter,
		ID:          id,
		Maximum:     maximum,
		Usage:       *usage,
	}, nil
}

func decodeScalar(raw json.RawMessage, nullable bool) (*string, error) {
	value := bytes.TrimSpace(raw)
	if bytes.Equal(value, []byte("null")) || len(value) == 0 {
		if nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("expected a string or number, got null")
	}

	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode scalar: %w", err)
	}

	var scalar string
	switch typedValue := decoded.(type) {
	case string:
		scalar = strings.TrimSpace(typedValue)
	case json.Number:
		scalar = typedValue.String()
	default:
		return nil, fmt.Errorf(
			"expected a string or number, got %T",
			decoded,
		)
	}
	if scalar == "" {
		return nil, fmt.Errorf("expected a non-empty string or number")
	}

	return &scalar, nil
}
