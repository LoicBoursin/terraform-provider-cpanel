package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

const (
	operationDeleteFilter  = "delete_filter"
	operationDisableFilter = "disable_filter"
	operationEnableFilter  = "enable_filter"
	operationGetFilter     = "get_filter"
	operationListFilters   = "list_filters"
	operationStoreFilter   = "store_filter"
)

type Filter struct {
	Account string
	Name    string
	Enabled bool
	Rules   []FilterRule
	Actions []FilterAction
}

type FilterRule struct {
	Part     string
	Match    string
	Value    string
	Operator string
}

type FilterAction struct {
	Action      string
	Destination string
}

type filterListResponse struct {
	Data []apiFilter `json:"data"`
}

type filterDetailResponse struct {
	Data apiFilterDetail `json:"data"`
}

type filterMutationResponse struct {
	Data json.RawMessage `json:"data"`
}

type apiFilter struct {
	Name       string            `json:"filtername"`
	EnabledRaw json.RawMessage   `json:"enabled"`
	Rules      []apiFilterRule   `json:"rules"`
	Actions    []apiFilterAction `json:"actions"`
}

type apiFilterDetail struct {
	Name    string                    `json:"filtername"`
	Rules   []apiNumberedFilterRule   `json:"rules"`
	Actions []apiNumberedFilterAction `json:"actions"`
}

type apiFilterRule struct {
	Part        string          `json:"part"`
	Match       string          `json:"match"`
	Value       string          `json:"val"`
	OperatorRaw json.RawMessage `json:"opt"`
}

type apiFilterAction struct {
	Action         string          `json:"action"`
	DestinationRaw json.RawMessage `json:"dest"`
}

type apiNumberedFilterRule struct {
	Number      int             `json:"number"`
	Part        string          `json:"part"`
	Match       string          `json:"match"`
	Value       string          `json:"val"`
	OperatorRaw json.RawMessage `json:"opt"`
}

type apiNumberedFilterAction struct {
	Number         int             `json:"number"`
	Action         string          `json:"action"`
	DestinationRaw json.RawMessage `json:"dest"`
}

func (c *Client) ListFilters(
	ctx context.Context,
	account string,
) ([]Filter, error) {
	if err := validateFilterAccount(account); err != nil {
		return nil, err
	}

	response := filterListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListFilters,
		map[string]string{"account": account},
		&response,
	); err != nil {
		return nil, err
	}

	filters := make([]Filter, 0, len(response.Data))
	seenNames := make(map[string]struct{}, len(response.Data))
	for _, apiValue := range response.Data {
		filter, err := normalizeAPIFilter(account, apiValue)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seenNames[filter.Name]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate email filter %q for account %q",
				filter.Name,
				account,
			)
		}
		seenNames[filter.Name] = struct{}{}
		filters = append(filters, filter)
	}

	return filters, nil
}

func (c *Client) GetFilter(
	ctx context.Context,
	account string,
	name string,
) (*Filter, error) {
	if err := validateFilterName(name); err != nil {
		return nil, err
	}

	filters, err := c.ListFilters(ctx, account)
	if err != nil {
		return nil, err
	}

	var inventoryFilter *Filter
	for index := range filters {
		if filters[index].Name == name {
			inventoryFilter = &filters[index]
			break
		}
	}
	if inventoryFilter == nil {
		return nil, nil
	}

	response := filterDetailResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationGetFilter,
		map[string]string{
			"account":    account,
			"filtername": name,
		},
		&response,
	); err != nil {
		return nil, err
	}
	if response.Data.Name != name {
		return nil, fmt.Errorf(
			"cPanel returned email filter %q when %q was requested for account %q",
			response.Data.Name,
			name,
			account,
		)
	}

	rules, err := normalizeNumberedFilterRules(name, response.Data.Rules)
	if err != nil {
		return nil, err
	}
	actions, err := normalizeNumberedFilterActions(
		account,
		name,
		response.Data.Actions,
	)
	if err != nil {
		return nil, err
	}
	if !slices.Equal(rules, inventoryFilter.Rules) ||
		!slices.Equal(actions, inventoryFilter.Actions) {
		return nil, fmt.Errorf(
			"email filter %q detail does not match the list_filters inventory",
			name,
		)
	}

	return &Filter{
		Account: account,
		Name:    name,
		Enabled: inventoryFilter.Enabled,
		Rules:   rules,
		Actions: actions,
	}, nil
}

func (c *Client) StoreFilter(
	ctx context.Context,
	account string,
	oldName string,
	filter Filter,
) error {
	if err := validateFilterAccount(account); err != nil {
		return err
	}
	if err := validateFilterDefinition(filter); err != nil {
		return err
	}

	parameters := map[string]string{
		"account":    account,
		"filtername": filter.Name,
	}
	if oldName != "" {
		parameters["oldfiltername"] = oldName
	}
	for index, rule := range filter.Rules {
		number := strconv.Itoa(index + 1)
		parameters["part"+number] = rule.Part
		parameters["match"+number] = rule.Match
		parameters["val"+number] = rule.Value
		if rule.Operator != "" {
			parameters["opt"+number] = rule.Operator
		}
	}
	for index, action := range filter.Actions {
		number := strconv.Itoa(index + 1)
		parameters["action"+number] = action.Action
		if action.Destination != "" {
			parameters["dest"+number] = action.Destination
		}
	}

	response := filterMutationResponse{}

	return c.executeMutation(
		ctx,
		operationStoreFilter,
		parameters,
		&response,
	)
}

func (c *Client) SetFilterEnabled(
	ctx context.Context,
	account string,
	name string,
	enabled bool,
) error {
	if err := validateFilterAccount(account); err != nil {
		return err
	}
	if err := validateFilterName(name); err != nil {
		return err
	}

	operation := operationDisableFilter
	if enabled {
		operation = operationEnableFilter
	}

	response := filterMutationResponse{}

	return c.executeMutation(
		ctx,
		operation,
		map[string]string{
			"account":    account,
			"filtername": name,
		},
		&response,
	)
}

func (c *Client) DeleteFilter(
	ctx context.Context,
	account string,
	name string,
) error {
	if err := validateFilterAccount(account); err != nil {
		return err
	}
	if err := validateFilterName(name); err != nil {
		return err
	}

	response := filterMutationResponse{}

	return c.executeMutation(
		ctx,
		operationDeleteFilter,
		map[string]string{
			"account":    account,
			"filtername": name,
		},
		&response,
	)
}

func normalizeAPIFilter(account string, apiValue apiFilter) (Filter, error) {
	if err := validateFilterName(apiValue.Name); err != nil {
		return Filter{}, fmt.Errorf(
			"invalid email filter inventory entry for account %q: %w",
			account,
			err,
		)
	}

	enabled, err := parseStrictFilterEnabled(apiValue.EnabledRaw)
	if err != nil {
		return Filter{}, fmt.Errorf(
			"email filter %q returned invalid enabled value: %w",
			apiValue.Name,
			err,
		)
	}
	rules, err := normalizeFilterRules(apiValue.Name, apiValue.Rules)
	if err != nil {
		return Filter{}, err
	}
	actions, err := normalizeFilterActions(
		account,
		apiValue.Name,
		apiValue.Actions,
	)
	if err != nil {
		return Filter{}, err
	}

	return Filter{
		Account: account,
		Name:    apiValue.Name,
		Enabled: enabled,
		Rules:   rules,
		Actions: actions,
	}, nil
}

func normalizeFilterRules(
	filterName string,
	apiRules []apiFilterRule,
) ([]FilterRule, error) {
	if len(apiRules) == 0 {
		return nil, fmt.Errorf(
			"email filter %q returned no rules",
			filterName,
		)
	}

	rules := make([]FilterRule, 0, len(apiRules))
	for index, apiRule := range apiRules {
		rule, err := normalizeFilterRule(
			filterName,
			index+1,
			apiRule.Part,
			apiRule.Match,
			apiRule.Value,
			apiRule.OperatorRaw,
		)
		if err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}

	return rules, nil
}

func normalizeFilterActions(
	account string,
	filterName string,
	apiActions []apiFilterAction,
) ([]FilterAction, error) {
	if len(apiActions) == 0 {
		return nil, fmt.Errorf(
			"email filter %q returned no actions",
			filterName,
		)
	}

	actions := make([]FilterAction, 0, len(apiActions))
	for index, apiAction := range apiActions {
		action, err := normalizeFilterAction(
			account,
			filterName,
			index+1,
			apiAction.Action,
			apiAction.DestinationRaw,
		)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}

	return actions, nil
}

func normalizeNumberedFilterRules(
	filterName string,
	apiRules []apiNumberedFilterRule,
) ([]FilterRule, error) {
	if len(apiRules) == 0 {
		return nil, fmt.Errorf(
			"email filter %q detail returned no rules",
			filterName,
		)
	}

	rules := make([]FilterRule, len(apiRules))
	seenNumbers := make([]bool, len(apiRules))
	for _, apiRule := range apiRules {
		if apiRule.Number < 1 || apiRule.Number > len(apiRules) {
			return nil, fmt.Errorf(
				"email filter %q returned rule number %d; expected consecutive numbers 1 through %d",
				filterName,
				apiRule.Number,
				len(apiRules),
			)
		}
		if seenNumbers[apiRule.Number-1] {
			return nil, fmt.Errorf(
				"email filter %q returned duplicate rule number %d",
				filterName,
				apiRule.Number,
			)
		}

		rule, err := normalizeFilterRule(
			filterName,
			apiRule.Number,
			apiRule.Part,
			apiRule.Match,
			apiRule.Value,
			apiRule.OperatorRaw,
		)
		if err != nil {
			return nil, err
		}
		seenNumbers[apiRule.Number-1] = true
		rules[apiRule.Number-1] = rule
	}

	return rules, nil
}

func normalizeNumberedFilterActions(
	account string,
	filterName string,
	apiActions []apiNumberedFilterAction,
) ([]FilterAction, error) {
	if len(apiActions) == 0 {
		return nil, fmt.Errorf(
			"email filter %q detail returned no actions",
			filterName,
		)
	}

	actions := make([]FilterAction, len(apiActions))
	seenNumbers := make([]bool, len(apiActions))
	for _, apiAction := range apiActions {
		if apiAction.Number < 1 || apiAction.Number > len(apiActions) {
			return nil, fmt.Errorf(
				"email filter %q returned action number %d; expected consecutive numbers 1 through %d",
				filterName,
				apiAction.Number,
				len(apiActions),
			)
		}
		if seenNumbers[apiAction.Number-1] {
			return nil, fmt.Errorf(
				"email filter %q returned duplicate action number %d",
				filterName,
				apiAction.Number,
			)
		}

		action, err := normalizeFilterAction(
			account,
			filterName,
			apiAction.Number,
			apiAction.Action,
			apiAction.DestinationRaw,
		)
		if err != nil {
			return nil, err
		}
		seenNumbers[apiAction.Number-1] = true
		actions[apiAction.Number-1] = action
	}

	return actions, nil
}

func normalizeFilterRule(
	filterName string,
	number int,
	part string,
	match string,
	value string,
	operatorRaw json.RawMessage,
) (FilterRule, error) {
	if part == "" {
		return FilterRule{}, fmt.Errorf(
			"email filter %q rule %d returned an empty part",
			filterName,
			number,
		)
	}
	if match == "" {
		if value != "" {
			return FilterRule{}, fmt.Errorf(
				"email filter %q rule %d returned an empty match with a non-empty value",
				filterName,
				number,
			)
		}
	} else if value == "" {
		return FilterRule{}, fmt.Errorf(
			"email filter %q rule %d returned a non-empty match with an empty value",
			filterName,
			number,
		)
	}

	operator, err := parseNullableFilterString(operatorRaw, true)
	if err != nil {
		return FilterRule{}, fmt.Errorf(
			"email filter %q rule %d returned invalid opt: %w",
			filterName,
			number,
			err,
		)
	}

	return FilterRule{
		Part:     part,
		Match:    match,
		Value:    value,
		Operator: operator,
	}, nil
}

func normalizeFilterAction(
	account string,
	filterName string,
	number int,
	action string,
	destinationRaw json.RawMessage,
) (FilterAction, error) {
	if action == "" {
		return FilterAction{}, fmt.Errorf(
			"email filter %q action %d returned an empty action",
			filterName,
			number,
		)
	}

	destination, err := parseNullableFilterString(
		destinationRaw,
		action == "finish",
	)
	if err != nil {
		return FilterAction{}, fmt.Errorf(
			"email filter %q action %d returned invalid dest: %w",
			filterName,
			number,
			err,
		)
	}
	if action == "save" {
		destination = normalizeSaveFilterDestination(account, destination)
	}

	return FilterAction{
		Action:      action,
		Destination: destination,
	}, nil
}

func normalizeSaveFilterDestination(account, destination string) string {
	user, domain, found := strings.Cut(account, "@")
	if !found || user == "" || domain == "" {
		return destination
	}

	mailboxPath := "$home/mail/" + domain + "/" + user
	if !strings.HasPrefix(destination, mailboxPath+"/") {
		return destination
	}

	return strings.TrimPrefix(destination, mailboxPath)
}

func parseStrictFilterEnabled(raw json.RawMessage) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return false, fmt.Errorf("value is missing")
	}
	if bytes.Equal(value, []byte("null")) {
		return false, fmt.Errorf("value is null")
	}

	switch string(value) {
	case "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	}

	var stringValue string
	if err := json.Unmarshal(value, &stringValue); err != nil {
		return false, fmt.Errorf(
			"expected 0, 1, a boolean, or an equivalent string",
		)
	}
	switch stringValue {
	case "0", "false":
		return false, nil
	case "1", "true":
		return true, nil
	default:
		return false, fmt.Errorf(
			"expected 0, 1, false, or true, got %q",
			stringValue,
		)
	}
}

func parseNullableFilterString(
	raw json.RawMessage,
	nullStringIsEmpty bool,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "", nil
	}
	if bytes.Equal(value, []byte("null")) {
		return "", nil
	}

	var stringValue string
	if err := json.Unmarshal(value, &stringValue); err != nil {
		return "", fmt.Errorf("expected a string or null")
	}
	if stringValue == "" || (nullStringIsEmpty && stringValue == "null") {
		return "", nil
	}

	return stringValue, nil
}

func validateFilterDefinition(filter Filter) error {
	if err := validateFilterName(filter.Name); err != nil {
		return err
	}
	if len(filter.Rules) == 0 {
		return fmt.Errorf("email filter %q must contain at least one rule", filter.Name)
	}
	for index, rule := range filter.Rules {
		if rule.Part == "" {
			return fmt.Errorf(
				"email filter %q rule %d part must not be empty",
				filter.Name,
				index+1,
			)
		}
		if rule.Match == "" && rule.Value != "" {
			return fmt.Errorf(
				"email filter %q rule %d match and value must both be empty or both be set",
				filter.Name,
				index+1,
			)
		}
		if rule.Match != "" && rule.Value == "" {
			return fmt.Errorf(
				"email filter %q rule %d match and value must both be empty or both be set",
				filter.Name,
				index+1,
			)
		}
	}
	if len(filter.Actions) == 0 {
		return fmt.Errorf(
			"email filter %q must contain at least one action",
			filter.Name,
		)
	}
	for index, action := range filter.Actions {
		if action.Action == "" {
			return fmt.Errorf(
				"email filter %q action %d must not be empty",
				filter.Name,
				index+1,
			)
		}
	}

	return nil
}

func IsMatchlessFilterPart(part string) bool {
	return part == "not delivered" || part == "error_message"
}

func validateFilterAccount(account string) error {
	if account == "" {
		return fmt.Errorf("email filter account must not be empty")
	}

	return nil
}

func validateFilterName(name string) error {
	if name == "" {
		return fmt.Errorf("email filter name must not be empty")
	}

	return nil
}
