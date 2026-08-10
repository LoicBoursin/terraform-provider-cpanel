package calendar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	moduleMu sync.Mutex
	module   string

	delegateLocksMu sync.Mutex
	delegateLocks   map[delegateIdentity]*delegateLock
}

type moduleResolution struct {
	Module          string
	Delegates       []Delegate
	InventoryLoaded bool
}

type delegateIdentity struct {
	Delegator string
	Delegatee string
	Calendar  string
}

type delegateLock struct {
	mutex      sync.Mutex
	references int
}

type objectEntry struct {
	Key   string
	Value json.RawMessage
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:        client,
		delegateLocks: make(map[delegateIdentity]*delegateLock),
	}
}

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	resolution, err := c.resolveModule(ctx)
	if err != nil {
		return nil, err
	}

	response := listUsersResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		resolution.Module,
		operationListUsers,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return normalizeAPIUsers(resolution.Module, response.Data)
}

func (c *Client) ListDelegates(ctx context.Context) ([]Delegate, error) {
	resolution, err := c.resolveModule(ctx)
	if err != nil {
		return nil, err
	}
	if resolution.InventoryLoaded {
		return resolution.Delegates, nil
	}

	return c.listDelegatesWithModule(ctx, resolution.Module)
}

func (c *Client) GetDelegate(
	ctx context.Context,
	delegator string,
	calendar string,
	delegatee string,
) (*Delegate, error) {
	identity, err := validateDelegateIdentity(
		delegator,
		calendar,
		delegatee,
	)
	if err != nil {
		return nil, err
	}

	delegates, err := c.ListDelegates(ctx)
	if err != nil {
		return nil, err
	}
	for index := range delegates {
		if delegateIdentityFromDelegate(delegates[index]) == identity {
			return &delegates[index], nil
		}
	}

	return nil, nil
}

func (c *Client) CalendarExists(
	ctx context.Context,
	username string,
	calendar string,
) (bool, error) {
	if err := validateRequiredInput("calendar user", username); err != nil {
		return false, err
	}
	if err := validateRequiredInput("calendar", calendar); err != nil {
		return false, err
	}

	users, err := c.ListUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, user := range users {
		if user.Username != username {
			continue
		}
		for _, collection := range user.Collections {
			if collection.Name == calendar &&
				collection.Type == "VCALENDAR" {
				return true, nil
			}
		}

		return false, nil
	}

	return false, nil
}

func (c *Client) AddDelegate(
	ctx context.Context,
	definition Definition,
) error {
	return c.mutateDelegate(
		ctx,
		operationAddDelegate,
		definition,
		true,
	)
}

func (c *Client) UpdateDelegate(
	ctx context.Context,
	definition Definition,
) error {
	return c.mutateDelegate(
		ctx,
		operationUpdateDelegate,
		definition,
		true,
	)
}

func (c *Client) RemoveDelegate(
	ctx context.Context,
	delegator string,
	calendar string,
	delegatee string,
) error {
	definition := Definition{
		Delegator: delegator,
		Delegatee: delegatee,
		Calendar:  calendar,
	}

	return c.mutateDelegate(
		ctx,
		operationRemoveDelegate,
		definition,
		false,
	)
}

func (c *Client) LockDelegate(
	delegator string,
	calendar string,
	delegatee string,
) func() {
	identity := delegateIdentity{
		Delegator: delegator,
		Delegatee: delegatee,
		Calendar:  calendar,
	}

	c.delegateLocksMu.Lock()
	if c.delegateLocks == nil {
		c.delegateLocks = make(map[delegateIdentity]*delegateLock)
	}
	lock := c.delegateLocks[identity]
	if lock == nil {
		lock = &delegateLock{}
		c.delegateLocks[identity] = lock
	}
	lock.references++
	c.delegateLocksMu.Unlock()

	lock.mutex.Lock()

	var once sync.Once

	return func() {
		once.Do(func() {
			lock.mutex.Unlock()

			c.delegateLocksMu.Lock()
			lock.references--
			if lock.references == 0 {
				delete(c.delegateLocks, identity)
			}
			c.delegateLocksMu.Unlock()
		})
	}
}

func (c *Client) resolveModule(
	ctx context.Context,
) (moduleResolution, error) {
	c.moduleMu.Lock()
	defer c.moduleMu.Unlock()

	if c.module != "" {
		return moduleResolution{Module: c.module}, nil
	}

	delegates, err := c.listDelegatesWithModule(
		ctx,
		cpanel.ModuleCPDAVD,
	)
	if err == nil {
		c.module = cpanel.ModuleCPDAVD

		return moduleResolution{
			Module:          c.module,
			Delegates:       delegates,
			InventoryLoaded: true,
		}, nil
	}
	if !isModuleUnavailable(
		err,
		cpanel.ModuleCPDAVD,
		operationListDelegates,
	) {
		return moduleResolution{}, fmt.Errorf(
			"probe CPDAVD calendar delegation module: %w",
			err,
		)
	}
	cpdavdError := err

	delegates, err = c.listDelegatesWithModule(ctx, cpanel.ModuleCCS)
	if err != nil {
		return moduleResolution{}, fmt.Errorf(
			"resolve calendar delegation UAPI module: "+
				"CPDAVD is unavailable (%v); CCS probe failed: %w",
			cpdavdError,
			err,
		)
	}

	c.module = cpanel.ModuleCCS

	return moduleResolution{
		Module:          c.module,
		Delegates:       delegates,
		InventoryLoaded: true,
	}, nil
}

func (c *Client) listDelegatesWithModule(
	ctx context.Context,
	module string,
) ([]Delegate, error) {
	response := listDelegatesResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		module,
		operationListDelegates,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return normalizeAPIDelegates(module, response.Data)
}

func (c *Client) mutateDelegate(
	ctx context.Context,
	operation string,
	definition Definition,
	includeReadOnly bool,
) error {
	if err := validateDefinition(definition); err != nil {
		return err
	}

	resolution, err := c.resolveModule(ctx)
	if err != nil {
		return err
	}

	parameters, err := delegateMutationParameters(
		resolution.Module,
		definition,
		includeReadOnly,
	)
	if err != nil {
		return err
	}

	response := mutationResponse{}

	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		resolution.Module,
		operation,
		parameters,
		&response,
	)
}

func delegateMutationParameters(
	module string,
	definition Definition,
	includeReadOnly bool,
) (map[string]string, error) {
	parameters := map[string]string{
		"delegator": definition.Delegator,
		"delegatee": definition.Delegatee,
	}
	if includeReadOnly {
		parameters["readonly"] = booleanParameter(definition.ReadOnly)
	}

	switch module {
	case cpanel.ModuleCPDAVD:
		parameters["calendar"] = definition.Calendar
	case cpanel.ModuleCCS:
		if definition.Calendar != DefaultCalendar {
			return nil, fmt.Errorf(
				"CCS supports only the default calendar %q, got %q",
				DefaultCalendar,
				definition.Calendar,
			)
		}
	default:
		return nil, fmt.Errorf(
			"unsupported calendar delegation module %q",
			module,
		)
	}

	return parameters, nil
}

func normalizeAPIDelegates(
	module string,
	raw json.RawMessage,
) ([]Delegate, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf(
			"%s::%s returned missing or null data",
			module,
			operationListDelegates,
		)
	}
	if value[0] != '[' {
		return nil, fmt.Errorf(
			"%s::%s data must be an array",
			module,
			operationListDelegates,
		)
	}

	var apiDelegates []apiDelegate
	if err := json.Unmarshal(value, &apiDelegates); err != nil {
		return nil, fmt.Errorf(
			"decode %s::%s data: %w",
			module,
			operationListDelegates,
			err,
		)
	}

	delegates := make([]Delegate, 0, len(apiDelegates))
	seen := make(map[delegateIdentity]struct{}, len(apiDelegates))
	for index, apiValue := range apiDelegates {
		delegate, err := normalizeAPIDelegate(module, apiValue)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid calendar delegate at index %d: %w",
				index,
				err,
			)
		}

		identity := delegateIdentityFromDelegate(delegate)
		if _, duplicate := seen[identity]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate calendar delegation "+
					"%q -> %q for calendar %q",
				delegate.Delegator,
				delegate.Delegatee,
				delegate.Calendar,
			)
		}
		seen[identity] = struct{}{}
		delegates = append(delegates, delegate)
	}

	sort.Slice(delegates, func(left, right int) bool {
		leftIdentity := delegateIdentityFromDelegate(delegates[left])
		rightIdentity := delegateIdentityFromDelegate(delegates[right])
		if leftIdentity.Delegator != rightIdentity.Delegator {
			return leftIdentity.Delegator < rightIdentity.Delegator
		}
		if leftIdentity.Delegatee != rightIdentity.Delegatee {
			return leftIdentity.Delegatee < rightIdentity.Delegatee
		}

		return leftIdentity.Calendar < rightIdentity.Calendar
	})

	return delegates, nil
}

func normalizeAPIDelegate(
	module string,
	apiValue apiDelegate,
) (Delegate, error) {
	if err := validateReturnedValue("delegator", apiValue.Delegator); err != nil {
		return Delegate{}, err
	}
	if err := validateReturnedValue("delegatee", apiValue.Delegatee); err != nil {
		return Delegate{}, err
	}

	calendar := apiValue.Calendar
	calendarName := apiValue.CalendarName
	readOnlyValue := apiValue.ReadOnly
	switch module {
	case cpanel.ModuleCPDAVD:
		if err := validateReturnedValue("calendar", calendar); err != nil {
			return Delegate{}, err
		}
		if err := validateReturnedValue(
			"calendar name",
			calendarName,
		); err != nil {
			return Delegate{}, err
		}
	case cpanel.ModuleCCS:
		calendar = DefaultCalendar
		calendarName = ""
		readOnlyValue = apiValue.LegacyReadOnly
	default:
		return Delegate{}, fmt.Errorf(
			"unsupported calendar delegation module %q",
			module,
		)
	}

	readOnly, err := parseRequiredBooleanFlag(readOnlyValue)
	if err != nil {
		return Delegate{}, fmt.Errorf("invalid readonly value: %w", err)
	}

	return Delegate{
		Delegator:    apiValue.Delegator,
		Delegatee:    apiValue.Delegatee,
		Calendar:     calendar,
		CalendarName: calendarName,
		ReadOnly:     readOnly,
	}, nil
}

func normalizeAPIUsers(module string, raw json.RawMessage) ([]User, error) {
	switch module {
	case cpanel.ModuleCPDAVD:
		return normalizeCPDAVDUsers(raw)
	case cpanel.ModuleCCS:
		return normalizeCCSUsers(raw)
	default:
		return nil, fmt.Errorf(
			"unsupported calendar delegation module %q",
			module,
		)
	}
}

func normalizeCPDAVDUsers(raw json.RawMessage) ([]User, error) {
	userEntries, err := decodeJSONObject(raw, "calendar user inventory")
	if err != nil {
		return nil, err
	}

	users := make([]User, 0, len(userEntries))
	for _, userEntry := range userEntries {
		if err := validateReturnedValue(
			"calendar username",
			userEntry.Key,
		); err != nil {
			return nil, err
		}

		collectionEntries, err := decodeJSONObject(
			userEntry.Value,
			fmt.Sprintf(
				"calendar collection inventory for user %q",
				userEntry.Key,
			),
		)
		if err != nil {
			return nil, err
		}

		collections := make([]Collection, 0, len(collectionEntries))
		for _, collectionEntry := range collectionEntries {
			if err := validateReturnedValue(
				"calendar collection name",
				collectionEntry.Key,
			); err != nil {
				return nil, err
			}

			apiValue := apiCollection{}
			if err := json.Unmarshal(
				collectionEntry.Value,
				&apiValue,
			); err != nil {
				return nil, fmt.Errorf(
					"decode calendar collection %q for user %q: %w",
					collectionEntry.Key,
					userEntry.Key,
					err,
				)
			}
			if err := validateReturnedValue(
				"calendar collection type",
				apiValue.Type,
			); err != nil {
				return nil, fmt.Errorf(
					"collection %q for user %q: %w",
					collectionEntry.Key,
					userEntry.Key,
					err,
				)
			}
			if apiValue.DisplayName != "" {
				if err := validateReturnedValue(
					"calendar collection display name",
					apiValue.DisplayName,
				); err != nil {
					return nil, fmt.Errorf(
						"collection %q for user %q: %w",
						collectionEntry.Key,
						userEntry.Key,
						err,
					)
				}
			}

			collections = append(collections, Collection{
				Name:        collectionEntry.Key,
				DisplayName: apiValue.DisplayName,
				Type:        apiValue.Type,
			})
		}
		sort.Slice(collections, func(left, right int) bool {
			return collections[left].Name < collections[right].Name
		})

		users = append(users, User{
			Username:    userEntry.Key,
			Collections: collections,
		})
	}
	sort.Slice(users, func(left, right int) bool {
		return users[left].Username < users[right].Username
	})

	return users, nil
}

func normalizeCCSUsers(raw json.RawMessage) ([]User, error) {
	userEntries, err := decodeJSONObject(raw, "calendar user inventory")
	if err != nil {
		return nil, err
	}

	users := make([]User, 0, len(userEntries))
	for _, userEntry := range userEntries {
		if err := validateReturnedValue(
			"calendar username",
			userEntry.Key,
		); err != nil {
			return nil, err
		}

		var userID string
		if err := json.Unmarshal(userEntry.Value, &userID); err != nil {
			return nil, fmt.Errorf(
				"decode legacy CCS calendar user identifier for %q: %w",
				userEntry.Key,
				err,
			)
		}
		if err := validateReturnedValue(
			"legacy CCS calendar user identifier",
			userID,
		); err != nil {
			return nil, fmt.Errorf(
				"calendar user %q: %w",
				userEntry.Key,
				err,
			)
		}

		users = append(users, User{
			Username: userEntry.Key,
			Collections: []Collection{{
				Name: DefaultCalendar,
				Type: "VCALENDAR",
			}},
		})
	}
	sort.Slice(users, func(left, right int) bool {
		return users[left].Username < users[right].Username
	})

	return users, nil
}

func decodeJSONObject(
	raw json.RawMessage,
	description string,
) ([]objectEntry, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("%s is missing or null", description)
	}

	decoder := json.NewDecoder(bytes.NewReader(value))
	opening, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", description, err)
	}
	if opening != json.Delim('{') {
		return nil, fmt.Errorf("%s must be an object", description)
	}

	entries := make([]objectEntry, 0)
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("decode %s key: %w", description, err)
		}
		key, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("%s returned a non-string key", description)
		}
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf(
				"%s returned duplicate key %q",
				description,
				key,
			)
		}
		seen[key] = struct{}{}

		var entryValue json.RawMessage
		if err := decoder.Decode(&entryValue); err != nil {
			return nil, fmt.Errorf(
				"decode %s value for key %q: %w",
				description,
				key,
				err,
			)
		}
		entries = append(entries, objectEntry{
			Key:   key,
			Value: entryValue,
		})
	}

	closing, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("decode %s closing token: %w", description, err)
	}
	if closing != json.Delim('}') {
		return nil, fmt.Errorf("%s did not end with an object", description)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, fmt.Errorf(
				"decode trailing %s data: %w",
				description,
				err,
			)
		}

		return nil, fmt.Errorf("%s contains trailing data", description)
	}

	return entries, nil
}

func validateDefinition(definition Definition) error {
	_, err := validateDelegateIdentity(
		definition.Delegator,
		definition.Calendar,
		definition.Delegatee,
	)

	return err
}

func validateDelegateIdentity(
	delegator string,
	calendar string,
	delegatee string,
) (delegateIdentity, error) {
	if err := validateRequiredInput("delegator", delegator); err != nil {
		return delegateIdentity{}, err
	}
	if err := validateRequiredInput("calendar", calendar); err != nil {
		return delegateIdentity{}, err
	}
	if err := validateRequiredInput("delegatee", delegatee); err != nil {
		return delegateIdentity{}, err
	}

	return delegateIdentity{
		Delegator: delegator,
		Delegatee: delegatee,
		Calendar:  calendar,
	}, nil
}

func validateRequiredInput(field string, value string) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf(
			"%s %q must not contain surrounding whitespace",
			field,
			value,
		)
	}

	return nil
}

func validateReturnedValue(field string, value string) error {
	if value == "" {
		return fmt.Errorf("cPanel returned an empty %s", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf(
			"cPanel returned %s %q with surrounding whitespace",
			field,
			value,
		)
	}

	return nil
}

func parseRequiredBooleanFlag(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return false, fmt.Errorf("value is missing or null")
	}

	switch string(value) {
	case "0", `"0"`:
		return false, nil
	case "1", `"1"`:
		return true, nil
	default:
		return false, fmt.Errorf("expected 0 or 1, got %s", value)
	}
}

func isModuleUnavailable(
	err error,
	module string,
	function string,
) bool {
	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) ||
		apiError.API != "UAPI" ||
		apiError.Module != module ||
		apiError.Function != function {
		return false
	}

	moduleName := strings.ToLower(module)
	modulePath := "cpanel/api/" + moduleName + ".pm"
	for _, message := range apiError.Messages {
		normalized := strings.ToLower(message)
		failedToLoad := strings.Contains(
			normalized,
			"failed to load module",
		) && strings.Contains(normalized, moduleName)
		moduleFailed := strings.Contains(normalized, "module") &&
			strings.Contains(normalized, moduleName) &&
			strings.Contains(normalized, "failed to load")
		missingModuleFile := strings.Contains(
			normalized,
			"can't locate "+modulePath,
		) || strings.Contains(
			normalized,
			"cannot locate "+modulePath,
		)
		if failedToLoad || moduleFailed || missingModuleFile {
			return true
		}
	}

	return false
}

func delegateIdentityFromDelegate(
	delegate Delegate,
) delegateIdentity {
	return delegateIdentity{
		Delegator: delegate.Delegator,
		Delegatee: delegate.Delegatee,
		Calendar:  delegate.Calendar,
	}
}

func booleanParameter(value bool) string {
	if value {
		return "1"
	}

	return "0"
}
