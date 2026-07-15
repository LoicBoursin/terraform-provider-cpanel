package locale

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"terraform-provider-cpanel/internal/cpanel"
)

var localeCodePattern = regexp.MustCompile(
	`^[a-z][a-z0-9]*(?:_[a-z0-9]+)*$`,
)

type Client struct {
	*cpanel.Client
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{Client: client}
}

func (c *Client) GetCurrent(ctx context.Context) (*Locale, error) {
	response := attributesResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleLocale,
		operationGetAttributes,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	code, err := normalizeCode(response.Data.Locale)
	if err != nil {
		return nil, fmt.Errorf("invalid current cPanel locale: %w", err)
	}
	direction, err := normalizeDirection(response.Data.Direction)
	if err != nil {
		return nil, fmt.Errorf(
			"locale %q returned invalid direction: %w",
			code,
			err,
		)
	}
	encoding := strings.ToLower(strings.TrimSpace(response.Data.Encoding))
	if encoding == "" {
		return nil, fmt.Errorf("locale %q returned an empty encoding", code)
	}

	current, err := c.Get(ctx, code)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, fmt.Errorf(
			"current locale %q is absent from the cPanel locale inventory",
			code,
		)
	}
	if current.Direction != direction {
		return nil, fmt.Errorf(
			"locale %q direction is %q in attributes and %q in the locale inventory",
			code,
			direction,
			current.Direction,
		)
	}
	current.Encoding = encoding

	return current, nil
}

func (c *Client) List(ctx context.Context) ([]Locale, error) {
	response := listResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleLocale,
		operationListLocales,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}
	if len(response.Data) == 0 {
		return nil, fmt.Errorf("cPanel returned an empty locale inventory")
	}

	locales := make([]Locale, 0, len(response.Data))
	seen := make(map[string]struct{}, len(response.Data))
	for _, apiValue := range response.Data {
		value, err := normalizeAPILocale(apiValue)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[value.Code]; exists {
			return nil, fmt.Errorf(
				"cPanel returned duplicate locale %q",
				value.Code,
			)
		}
		seen[value.Code] = struct{}{}
		locales = append(locales, value)
	}
	sort.Slice(locales, func(left, right int) bool {
		return locales[left].Code < locales[right].Code
	})

	return locales, nil
}

func (c *Client) Get(ctx context.Context, code string) (*Locale, error) {
	normalizedCode, err := normalizeCode(code)
	if err != nil {
		return nil, err
	}

	locales, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(locales), func(index int) bool {
		return locales[index].Code >= normalizedCode
	})
	if index == len(locales) || locales[index].Code != normalizedCode {
		return nil, nil
	}

	return &locales[index], nil
}

func (c *Client) Set(ctx context.Context, code string) (*Locale, error) {
	target, err := c.Get(ctx, code)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, fmt.Errorf(
			"locale %q is not available in the cPanel locale inventory",
			code,
		)
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleLocale,
		operationSetLocale,
		map[string]string{"locale": target.Code},
		&response,
	); err != nil {
		return nil, err
	}

	current, err := c.GetCurrent(ctx)
	if err != nil {
		return nil, fmt.Errorf("read locale after mutation: %w", err)
	}
	if current.Code != target.Code {
		return nil, fmt.Errorf(
			"cPanel locale is %q after mutation; expected %q",
			current.Code,
			target.Code,
		)
	}

	return current, nil
}

func ValidateCode(code string) error {
	_, err := normalizeCode(code)

	return err
}

func normalizeCode(code string) (string, error) {
	if code == "" {
		return "", fmt.Errorf("locale code must not be empty")
	}
	if strings.TrimSpace(code) != code {
		return "", fmt.Errorf(
			"locale code %q must not contain surrounding whitespace",
			code,
		)
	}
	if len(code) > 64 || !localeCodePattern.MatchString(code) {
		return "", fmt.Errorf(
			"locale code %q must contain lowercase letters, digits, and underscore-separated segments",
			code,
		)
	}

	return code, nil
}

func normalizeDirection(direction string) (string, error) {
	switch direction {
	case DirectionLeftToRight, DirectionRightToLeft:
		return direction, nil
	default:
		return "", fmt.Errorf("unsupported text direction %q", direction)
	}
}

func normalizeAPILocale(apiValue apiLocale) (Locale, error) {
	code, err := normalizeCode(apiValue.Locale)
	if err != nil {
		return Locale{}, fmt.Errorf("invalid cPanel locale inventory entry: %w", err)
	}
	direction, err := normalizeDirection(apiValue.Direction)
	if err != nil {
		return Locale{}, fmt.Errorf(
			"locale %q returned invalid direction: %w",
			code,
			err,
		)
	}
	name := strings.TrimSpace(apiValue.Name)
	if name == "" {
		return Locale{}, fmt.Errorf("locale %q returned an empty name", code)
	}
	localName := strings.TrimSpace(apiValue.LocalName)
	if localName == "" {
		return Locale{}, fmt.Errorf(
			"locale %q returned an empty local name",
			code,
		)
	}

	return Locale{
		Code:      code,
		Direction: direction,
		LocalName: localName,
		Name:      name,
	}, nil
}
