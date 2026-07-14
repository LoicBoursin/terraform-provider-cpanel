package cpanel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultRequestTimeout = 90 * time.Second
	maxResponseSize       = 10 << 20
)

var ErrResponseTooLarge = errors.New("cPanel API response exceeds 10 MiB")

type Client struct {
	HTTPClient *http.Client
	HostURL    string
	Auth       AuthStruct

	requestMu sync.Mutex
}

type AuthStruct struct {
	Username string
	APIToken string
}

type APIError struct {
	API      string
	Module   string
	Function string
	Messages []string
}

func (e *APIError) Error() string {
	message := "unknown cPanel API error"
	if len(e.Messages) > 0 {
		message = strings.Join(e.Messages, "; ")
	}

	return fmt.Sprintf("%s %s::%s failed: %s", e.API, e.Module, e.Function, message)
}

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("cPanel API request failed with HTTP %d", e.StatusCode)
}

func NewClient(host, username, apiToken string) (*Client, error) {
	parsedHost, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse cPanel host: %w", err)
	}

	if parsedHost.Scheme == "" || parsedHost.Host == "" {
		return nil, errors.New("cPanel host must be an absolute URL")
	}

	if parsedHost.Scheme != "https" && !isLoopbackHTTP(parsedHost) {
		return nil, errors.New("cPanel host must use HTTPS")
	}

	if parsedHost.User != nil || parsedHost.RawQuery != "" || parsedHost.Fragment != "" {
		return nil, errors.New("cPanel host must not contain credentials, a query, or a fragment")
	}

	parsedHost.Path = strings.TrimRight(parsedHost.Path, "/")

	return &Client{
		HTTPClient: &http.Client{Timeout: defaultRequestTimeout},
		HostURL:    strings.TrimRight(parsedHost.String(), "/"),
		Auth: AuthStruct{
			Username: username,
			APIToken: apiToken,
		},
	}, nil
}

func isLoopbackHTTP(parsedHost *url.URL) bool {
	if parsedHost.Scheme != "http" {
		return false
	}

	hostname := parsedHost.Hostname()
	if hostname == "localhost" {
		return true
	}

	ip := net.ParseIP(hostname)

	return ip != nil && ip.IsLoopback()
}

func (c *Client) ExecuteUAPIOperation(
	ctx context.Context,
	method string,
	module string,
	function string,
	parameters map[string]string,
	output any,
) error {
	endpoint := fmt.Sprintf("/execute/%s/%s", url.PathEscape(module), url.PathEscape(function))
	body, err := c.execute(ctx, method, endpoint, parameters)
	if err != nil {
		return err
	}

	var envelope struct {
		Status   int      `json:"status"`
		Errors   []string `json:"errors"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode UAPI response envelope: %w", err)
	}

	if envelope.Status != 1 {
		messages := append([]string{}, envelope.Errors...)
		messages = append(messages, envelope.Messages...)

		return &APIError{
			API:      "UAPI",
			Module:   module,
			Function: function,
			Messages: messages,
		}
	}

	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("decode UAPI response: %w", err)
	}

	return nil
}

func (c *Client) ExecuteAPI2Operation(
	ctx context.Context,
	method string,
	module string,
	function string,
	parameters map[string]string,
	output any,
) error {
	requestParameters := map[string]string{
		"cpanel_jsonapi_apiversion": "2",
		"cpanel_jsonapi_user":       c.Auth.Username,
		"cpanel_jsonapi_module":     module,
		"cpanel_jsonapi_func":       function,
	}
	for key, value := range parameters {
		requestParameters[key] = value
	}

	body, err := c.execute(ctx, method, "/json-api/cpanel", requestParameters)
	if err != nil {
		return err
	}

	var envelope struct {
		CpanelResult struct {
			Event struct {
				Result int `json:"result"`
			} `json:"event"`
			Data []map[string]any `json:"data"`
		} `json:"cpanelresult"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("decode API 2 response envelope: %w", err)
	}

	if envelope.CpanelResult.Event.Result != 1 {
		return &APIError{
			API:      "API 2",
			Module:   module,
			Function: function,
			Messages: api2Messages(envelope.CpanelResult.Data),
		}
	}

	if err := json.Unmarshal(body, output); err != nil {
		return fmt.Errorf("decode API 2 response: %w", err)
	}

	return nil
}

func api2Messages(data []map[string]any) []string {
	var messages []string

	for _, item := range data {
		for _, key := range []string{"reason", "statusmsg"} {
			message, ok := item[key].(string)
			if ok && message != "" {
				messages = append(messages, message)
			}
		}
	}

	return messages
}

func (c *Client) execute(
	ctx context.Context,
	method string,
	endpoint string,
	parameters map[string]string,
) ([]byte, error) {
	values := url.Values{}
	for key, value := range parameters {
		values.Set(key, value)
	}

	requestURL := c.HostURL + endpoint
	var body io.Reader

	switch method {
	case http.MethodGet:
		if encodedParameters := values.Encode(); encodedParameters != "" {
			requestURL += "?" + encodedParameters
		}
	case http.MethodPost:
		body = strings.NewReader(values.Encode())
	default:
		return nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, fmt.Errorf("create cPanel API request: %w", err)
	}

	request.Header.Set("Accept", "application/json")
	request.Header.Set(
		"Authorization",
		fmt.Sprintf("cpanel %s:%s", c.Auth.Username, c.Auth.APIToken),
	)
	request.Header.Set("User-Agent", "terraform-provider-cpanel")
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	c.requestMu.Lock()
	defer c.requestMu.Unlock()

	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("execute cPanel API request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("read cPanel API response: %w", err)
	}
	if len(responseBody) > maxResponseSize {
		return nil, ErrResponseTooLarge
	}

	if response.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: response.StatusCode}
	}

	return responseBody, nil
}
