package cpanel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	HTTPClient *http.Client
	HostURL    string
	Auth       AuthStruct
}

type AuthStruct struct {
	Username string `json:"username"`
	ApiToken string `json:"api_token"`
}

func NewClient(host, username, apiToken *string) (*Client, error) {
	c := Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		HostURL:    *host,
		Auth: AuthStruct{
			Username: *username,
			ApiToken: *apiToken,
		},
	}

	return &c, nil
}

func (c *Client) doRequest(req *http.Request) ([]byte, error) {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s:%s", "cpanel", c.Auth.Username, c.Auth.ApiToken))

	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(res.Body)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d, body: %s", res.StatusCode, body)
	}

	return body, err
}

func (c *Client) ExecuteUAPIOperation(module, function string, queryParams map[string]string, inputModel interface{}) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/execute/%s/%s", c.HostURL, module, function), nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	for key, value := range queryParams {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	body, err := c.doRequest(req)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, inputModel)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) ExecuteAPI2Operation(module, function string, queryParams map[string]string, inputModel interface{}) error {
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/json-api/cpanel?cpanel_jsonapi_apiversion=2&cpanel_jsonapi_user=%s&cpanel_jsonapi_module=%s&cpanel_jsonapi_func=%s", c.HostURL, c.Auth.Username, module, function), nil)
	if err != nil {
		return err
	}

	q := req.URL.Query()
	for key, value := range queryParams {
		q.Add(key, value)
	}
	req.URL.RawQuery = q.Encode()

	body, err := c.doRequest(req)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, inputModel)
	if err != nil {
		return err
	}

	return nil
}
