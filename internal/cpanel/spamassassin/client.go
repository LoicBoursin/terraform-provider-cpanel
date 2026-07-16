package spamassassin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	mutationMu sync.Mutex
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) LockPreference(_ string) func() {
	c.mutationMu.Lock()

	var once sync.Once

	return func() {
		once.Do(c.mutationMu.Unlock)
	}
}

func (c *Client) GetPreference(
	ctx context.Context,
	name string,
) (*Preference, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	response := preferencesResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleSpamAssassin,
		operationGetUserPreferences,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return preferenceFromResponse(name, response.Data)
}

func (c *Client) SetPreference(
	ctx context.Context,
	name string,
	values []string,
) (*Preference, error) {
	target := Definition{
		Name:    name,
		Values:  slices.Clone(values),
		Present: true,
	}.Sorted()
	if err := ValidateDefinition(target); err != nil {
		return nil, err
	}

	parameters := url.Values{"preference": {target.Name}}
	if len(target.Values) == 1 {
		parameters.Set("value", target.Values[0])
	} else {
		for index, value := range target.Values {
			parameters.Set(fmt.Sprintf("value-%d", index), value)
		}
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperationValues(
		ctx,
		http.MethodPost,
		cpanel.ModuleSpamAssassin,
		operationUpdateUserPreference,
		parameters,
		&response,
	); err != nil {
		return nil, err
	}

	actual, err := c.GetPreference(ctx, target.Name)
	if err != nil {
		return nil, fmt.Errorf(
			"read SpamAssassin preference %q after mutation: %w",
			target.Name,
			err,
		)
	}
	if !PreferenceMatchesDefinition(*actual, target) {
		return nil, fmt.Errorf(
			"cPanel SpamAssassin preference %q is %q after mutation; expected %q",
			target.Name,
			actual.Values,
			target.Values,
		)
	}

	return actual, nil
}

func (c *Client) RemovePreference(
	ctx context.Context,
	name string,
) (*Preference, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperationValues(
		ctx,
		http.MethodPost,
		cpanel.ModuleSpamAssassin,
		operationUpdateUserPreference,
		url.Values{"preference": {name}},
		&response,
	); err != nil {
		return nil, err
	}

	actual, err := c.GetPreference(ctx, name)
	if err != nil {
		return nil, fmt.Errorf(
			"read SpamAssassin preference %q after removal: %w",
			name,
			err,
		)
	}
	if actual.Present {
		return nil, fmt.Errorf(
			"cPanel SpamAssassin preference %q remains configured after removal",
			name,
		)
	}

	return actual, nil
}

func PreferenceMatchesDefinition(
	preference Preference,
	definition Definition,
) bool {
	actual := preference.Definition()
	expected := definition.Sorted()

	return actual.Name == expected.Name &&
		actual.Present == expected.Present &&
		slices.Equal(actual.Values, expected.Values)
}

func preferenceFromResponse(
	name string,
	rawData json.RawMessage,
) (*Preference, error) {
	dataBytes := bytes.TrimSpace(rawData)
	if len(dataBytes) == 0 || bytes.Equal(dataBytes, []byte("null")) ||
		dataBytes[0] != '{' {
		return nil, fmt.Errorf(
			"cPanel returned invalid SpamAssassin preference data; expected an object",
		)
	}

	data := map[string]json.RawMessage{}
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		return nil, fmt.Errorf(
			"decode cPanel SpamAssassin preference data: %w",
			err,
		)
	}
	rawValues, present := data[name]
	if !present {
		return &Preference{
			Name:    name,
			Values:  []string{},
			Present: false,
		}, nil
	}

	var values []string
	if err := json.Unmarshal(rawValues, &values); err != nil {
		return nil, fmt.Errorf(
			"cPanel returned invalid values for SpamAssassin preference %q: %w",
			name,
			err,
		)
	}
	preference := Preference{
		Name:    name,
		Values:  values,
		Present: true,
	}
	definition := preference.Definition()
	if err := ValidateDefinition(definition); err != nil {
		return nil, fmt.Errorf(
			"cPanel returned invalid SpamAssassin preference %q: %w",
			name,
			err,
		)
	}
	preference.Values = definition.Values

	return &preference, nil
}
