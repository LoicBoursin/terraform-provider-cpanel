package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	operationAddMailingList               = "add_list"
	operationChangeMailingListPassword    = "passwd_list"
	operationDeleteMailingList            = "delete_list"
	operationListMailingLists             = "list_lists"
	operationSetMailingListPrivacyOptions = "set_list_privacy_options"
)

type MailingList struct {
	Address         string
	ID              string
	Private         bool
	Advertised      bool
	ArchivePrivate  bool
	SubscribePolicy int64
	Administrators  []string
	HumanDiskUsed   string
}

type MailingListPrivacyOptions struct {
	Advertised      bool
	ArchivePrivate  bool
	SubscribePolicy int64
}

type mailingListResponse struct {
	Data json.RawMessage `json:"data"`
}

type mailingListMutationResponse struct {
	Data json.RawMessage `json:"data"`
}

type apiMailingList struct {
	Address         json.RawMessage `json:"list"`
	ID              json.RawMessage `json:"listid"`
	AccessType      json.RawMessage `json:"accesstype"`
	Advertised      json.RawMessage `json:"advertised"`
	ArchivePrivate  json.RawMessage `json:"archive_private"`
	SubscribePolicy json.RawMessage `json:"subscribe_policy"`
	Administrators  json.RawMessage `json:"listadmin"`
	HumanDiskUsed   json.RawMessage `json:"humandiskused"`
}

func (c *Client) ListMailingLists(
	ctx context.Context,
	domain string,
) ([]MailingList, error) {
	parameters := map[string]string{}
	if domain != "" {
		parameters["domain"] = domain
	}

	response := mailingListResponse{}
	if err := c.executeReadOperation(
		ctx,
		operationListMailingLists,
		parameters,
		&response,
	); err != nil {
		return nil, err
	}

	return normalizeMailingListInventory(response.Data)
}

func (c *Client) GetMailingList(
	ctx context.Context,
	address string,
	domain string,
) (*MailingList, error) {
	mailingLists, err := c.ListMailingLists(ctx, domain)
	if err != nil {
		return nil, err
	}

	for _, mailingList := range mailingLists {
		if mailingList.Address == address {
			mailingListCopy := mailingList

			return &mailingListCopy, nil
		}
	}

	return nil, nil
}

func (c *Client) CreateMailingList(
	ctx context.Context,
	user string,
	domain string,
	password string,
	private bool,
) error {
	privateFlag := "0"
	if private {
		privateFlag = "1"
	}

	response := mailingListMutationResponse{}

	return c.executeMutation(ctx, operationAddMailingList, map[string]string{
		"list":     user,
		"domain":   domain,
		"password": password,
		"private":  privateFlag,
	}, &response)
}

func (c *Client) DeleteMailingList(
	ctx context.Context,
	address string,
) error {
	response := mailingListMutationResponse{}

	return c.executeMutation(
		ctx,
		operationDeleteMailingList,
		map[string]string{"list": address},
		&response,
	)
}

func (c *Client) ChangeMailingListPassword(
	ctx context.Context,
	address string,
	password string,
) error {
	if err := validateMailingListIdentifier("address", address); err != nil {
		return err
	}
	if password == "" {
		return fmt.Errorf("mailing list password must not be empty")
	}

	response := mailingListMutationResponse{}

	return c.executeMutation(
		ctx,
		operationChangeMailingListPassword,
		map[string]string{
			"list":     address,
			"password": password,
		},
		&response,
	)
}

func (c *Client) SetMailingListPrivacyOptions(
	ctx context.Context,
	address string,
	options MailingListPrivacyOptions,
) error {
	if err := validateMailingListIdentifier("address", address); err != nil {
		return err
	}
	if options.SubscribePolicy < 1 || options.SubscribePolicy > 3 {
		return fmt.Errorf(
			"mailing list subscribe policy must be from 1 through 3, got %d",
			options.SubscribePolicy,
		)
	}

	response := mailingListMutationResponse{}

	return c.executeMutation(
		ctx,
		operationSetMailingListPrivacyOptions,
		map[string]string{
			"list":             address,
			"advertised":       mailingListBooleanFlag(options.Advertised),
			"archive_private":  mailingListBooleanFlag(options.ArchivePrivate),
			"subscribe_policy": strconv.FormatInt(options.SubscribePolicy, 10),
		},
		&response,
	)
}

func normalizeMailingListInventory(
	raw json.RawMessage,
) ([]MailingList, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("mailing list inventory data is missing or null")
	}
	if value[0] != '[' {
		return nil, fmt.Errorf("mailing list inventory data must be an array")
	}

	var apiMailingLists []apiMailingList
	if err := json.Unmarshal(value, &apiMailingLists); err != nil {
		return nil, fmt.Errorf("decode mailing list inventory data: %w", err)
	}

	mailingLists := make([]MailingList, 0, len(apiMailingLists))
	seenAddresses := make(map[string]struct{}, len(apiMailingLists))
	for index, apiValue := range apiMailingLists {
		mailingList, err := mailingListFromAPI(apiValue)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid mailing list at index %d: %w",
				index,
				err,
			)
		}
		if _, duplicate := seenAddresses[mailingList.Address]; duplicate {
			return nil, fmt.Errorf(
				"cPanel returned duplicate mailing list address %q",
				mailingList.Address,
			)
		}
		seenAddresses[mailingList.Address] = struct{}{}
		mailingLists = append(mailingLists, mailingList)
	}

	return mailingLists, nil
}

func mailingListFromAPI(value apiMailingList) (MailingList, error) {
	address, err := parseRequiredMailingListString(value.Address)
	if err != nil {
		return MailingList{}, fmt.Errorf("decode mailing list address: %w", err)
	}
	if err := validateMailingListIdentifier("address", address); err != nil {
		return MailingList{}, err
	}

	id, err := parseRequiredMailingListString(value.ID)
	if err != nil {
		return MailingList{}, fmt.Errorf(
			"decode mailing list %q id: %w",
			address,
			err,
		)
	}
	if err := validateMailingListIdentifier("id", id); err != nil {
		return MailingList{}, fmt.Errorf("mailing list %q: %w", address, err)
	}

	accessType, err := parseRequiredMailingListString(value.AccessType)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"accesstype",
			err,
		)
	}
	advertised, err := parseStrictMailingListBoolean(value.Advertised)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"advertised",
			err,
		)
	}
	archivePrivate, err := parseStrictMailingListBoolean(
		value.ArchivePrivate,
	)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"archive_private",
			err,
		)
	}
	subscribePolicy, err := parseStrictMailingListPolicy(
		value.SubscribePolicy,
	)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"subscribe_policy",
			err,
		)
	}
	administratorsValue, err := parseRequiredMailingListString(
		value.Administrators,
	)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"listadmin",
			err,
		)
	}
	humanDiskUsed, err := parseRequiredMailingListString(
		value.HumanDiskUsed,
	)
	if err != nil {
		return MailingList{}, mailingListFieldError(
			address,
			"humandiskused",
			err,
		)
	}

	private, err := validateMailingListAccess(
		address,
		accessType,
		advertised,
		archivePrivate,
		subscribePolicy,
	)
	if err != nil {
		return MailingList{}, err
	}

	return MailingList{
		Address:         address,
		ID:              id,
		Private:         private,
		Advertised:      advertised,
		ArchivePrivate:  archivePrivate,
		SubscribePolicy: subscribePolicy,
		Administrators:  parseMailingListAdministrators(administratorsValue),
		HumanDiskUsed:   humanDiskUsed,
	}, nil
}

func validateMailingListAccess(
	address string,
	accessType string,
	advertised bool,
	archivePrivate bool,
	subscribePolicy int64,
) (bool, error) {
	private := !advertised &&
		archivePrivate &&
		(subscribePolicy == 2 || subscribePolicy == 3)
	expectedAccessType := "public"
	if private {
		expectedAccessType = "private"
	}
	if accessType != expectedAccessType {
		return false, fmt.Errorf(
			"mailing list %q returned inconsistent access settings: "+
				"accesstype is %q, expected %q from advertised=%t, "+
				"archive_private=%t, and subscribe_policy=%d",
			address,
			accessType,
			expectedAccessType,
			advertised,
			archivePrivate,
			subscribePolicy,
		)
	}

	return private, nil
}

func parseRequiredMailingListString(
	raw json.RawMessage,
) (string, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return "", fmt.Errorf("required string value is missing or null")
	}
	if value[0] != '"' {
		return "", fmt.Errorf("expected a string, got %s", value)
	}

	var parsed string
	if err := json.Unmarshal(value, &parsed); err != nil {
		return "", fmt.Errorf("decode string: %w", err)
	}

	return parsed, nil
}

func parseStrictMailingListBoolean(
	raw json.RawMessage,
) (bool, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return false, fmt.Errorf("required boolean value is missing or null")
	}

	switch string(value) {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("expected integer 0 or 1, got %s", value)
	}
}

func parseStrictMailingListPolicy(
	raw json.RawMessage,
) (int64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("required integer value is missing or null")
	}

	var parsed int64
	if err := json.Unmarshal(value, &parsed); err != nil {
		return 0, fmt.Errorf("expected an integer from 1 through 3")
	}
	if parsed < 1 || parsed > 3 {
		return 0, fmt.Errorf(
			"expected an integer from 1 through 3, got %d",
			parsed,
		)
	}

	return parsed, nil
}

func parseMailingListAdministrators(value string) []string {
	if value == "" {
		return []string{}
	}

	administrators := make([]string, 0, strings.Count(value, ",")+1)
	for _, administrator := range strings.Split(value, ",") {
		administrator = strings.TrimSpace(administrator)
		if administrator != "" {
			administrators = append(administrators, administrator)
		}
	}
	sort.Strings(administrators)

	return administrators
}

func validateMailingListIdentifier(field string, value string) error {
	if value == "" {
		return fmt.Errorf("mailing list %s must not be empty", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf(
			"mailing list %s %q must not contain surrounding whitespace",
			field,
			value,
		)
	}

	return nil
}

func mailingListBooleanFlag(value bool) string {
	if value {
		return "1"
	}

	return "0"
}

func mailingListFieldError(
	address string,
	field string,
	err error,
) error {
	return fmt.Errorf(
		"decode mailing list %q %s: %w",
		address,
		field,
		err,
	)
}
