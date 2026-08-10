package boxtrapper

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	accountLocksMu sync.Mutex
	accountLocks   map[string]*accountLock
}

type accountLock struct {
	mutex      sync.Mutex
	references int
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:       client,
		accountLocks: make(map[string]*accountLock),
	}
}

func (c *Client) Get(
	ctx context.Context,
	account string,
) (*Settings, error) {
	if err := validateAccount(account); err != nil {
		return nil, err
	}

	accounts, err := c.listAccounts(ctx)
	if err != nil {
		return nil, err
	}
	index := sort.Search(len(accounts), func(index int) bool {
		return accounts[index].Account >= account
	})
	if index == len(accounts) || accounts[index].Account != account {
		return nil, nil
	}

	enabled, err := c.getStatus(ctx, account)
	if err != nil {
		return nil, fmt.Errorf(
			"read BoxTrapper status for account %q: %w",
			account,
			err,
		)
	}
	configuration, err := c.getConfiguration(ctx, account)
	if err != nil {
		return nil, fmt.Errorf(
			"read BoxTrapper configuration for account %q: %w",
			account,
			err,
		)
	}
	if accounts[index].Enabled != enabled {
		return nil, fmt.Errorf(
			"cPanel BoxTrapper status for account %q disagrees between API 2 (%t) and UAPI (%t)",
			account,
			accounts[index].Enabled,
			enabled,
		)
	}

	return &Settings{
		Account:       account,
		Enabled:       enabled,
		Configuration: configuration,
	}, nil
}

func (c *Client) SetStatus(
	ctx context.Context,
	account string,
	enabled bool,
) (*Settings, []string, error) {
	if err := validateAccount(account); err != nil {
		return nil, nil, err
	}

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleBoxTrapper,
		operationSetStatus,
		map[string]string{
			"email":   account,
			"enabled": booleanParameter(enabled),
		},
		&response,
	); err != nil {
		return nil, response.Warnings, err
	}

	actual, err := c.Get(ctx, account)
	if err != nil {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "status",
			Err: fmt.Errorf(
				"read BoxTrapper settings after mutation: %w",
				err,
			),
		}
	}
	if actual == nil {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "status",
			Err: fmt.Errorf(
				"cPanel BoxTrapper account %q disappeared after mutation",
				account,
			),
		}
	}
	if actual.Enabled != enabled {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "status",
			Err: fmt.Errorf(
				"cPanel BoxTrapper enabled status for account %q is %t after mutation; expected %t",
				account,
				actual.Enabled,
				enabled,
			),
		}
	}

	return actual, response.Warnings, nil
}

func (c *Client) SaveConfiguration(
	ctx context.Context,
	account string,
	definition Definition,
	fromName *string,
) (*Settings, []string, error) {
	if err := validateAccount(account); err != nil {
		return nil, nil, err
	}
	if err := ValidateDefinition(definition); err != nil {
		return nil, nil, err
	}
	if fromName == nil {
		return nil, nil, fmt.Errorf(
			"BoxTrapper configuration for account %q cannot be changed while cPanel reports from_name as null because save_configuration would replace null with an empty string",
			account,
		)
	}
	if err := validateManagedString(
		"BoxTrapper from_name",
		*fromName,
		false,
	); err != nil {
		return nil, nil, err
	}
	preservedFromName := *fromName

	response := mutationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleBoxTrapper,
		operationSaveConfiguration,
		map[string]string{
			"email":                    account,
			"enable_auto_whitelist":    booleanParameter(definition.EnableAutoWhitelist),
			"from_addresses":           definition.FromAddresses,
			"from_name":                preservedFromName,
			"queue_days":               strconv.FormatInt(definition.QueueDays, 10),
			"spam_score":               formatSpamScore(definition.SpamScore),
			"whitelist_by_association": booleanParameter(definition.WhitelistByAssociation),
		},
		&response,
	); err != nil {
		return nil, response.Warnings, err
	}

	actual, err := c.Get(ctx, account)
	if err != nil {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "configuration",
			Err: fmt.Errorf(
				"read BoxTrapper settings after mutation: %w",
				err,
			),
		}
	}
	if actual == nil {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "configuration",
			Err: fmt.Errorf(
				"cPanel BoxTrapper account %q disappeared after mutation",
				account,
			),
		}
	}
	if err := verifyConfiguration(
		actual.Configuration,
		definition,
	); err != nil {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "configuration",
			Err:       err,
		}
	}
	if actual.FromName == nil ||
		*actual.FromName != preservedFromName {
		return nil, response.Warnings, &MutationVerificationError{
			Operation: "configuration",
			Err: fmt.Errorf(
				"cPanel BoxTrapper from_name for account %q changed after mutation; expected %q",
				account,
				preservedFromName,
			),
		}
	}

	return actual, response.Warnings, nil
}

func (c *Client) LockAccount(account string) func() {
	c.accountLocksMu.Lock()
	if c.accountLocks == nil {
		c.accountLocks = make(map[string]*accountLock)
	}
	lock := c.accountLocks[account]
	if lock == nil {
		lock = &accountLock{}
		c.accountLocks[account] = lock
	}
	lock.references++
	c.accountLocksMu.Unlock()

	lock.mutex.Lock()

	var once sync.Once

	return func() {
		once.Do(func() {
			lock.mutex.Unlock()

			c.accountLocksMu.Lock()
			lock.references--
			if lock.references == 0 {
				delete(c.accountLocks, account)
			}
			c.accountLocksMu.Unlock()
		})
	}
}

func (c *Client) listAccounts(
	ctx context.Context,
) ([]accountInventoryEntry, error) {
	response := accountListResponse{}
	if err := c.ExecuteAPI2Operation(
		ctx,
		http.MethodGet,
		cpanel.ModuleBoxTrapper,
		operationAccountManageList,
		map[string]string{},
		&response,
	); err != nil {
		return nil, err
	}

	return response.Accounts, nil
}

func (c *Client) getStatus(
	ctx context.Context,
	account string,
) (bool, error) {
	response := statusResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleBoxTrapper,
		operationGetStatus,
		map[string]string{"email": account},
		&response,
	); err != nil {
		return false, err
	}

	return response.Enabled, nil
}

func (c *Client) getConfiguration(
	ctx context.Context,
	account string,
) (Configuration, error) {
	response := configurationResponse{}
	if err := c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleBoxTrapper,
		operationGetConfiguration,
		map[string]string{"email": account},
		&response,
	); err != nil {
		return Configuration{}, err
	}

	return response.Configuration, nil
}
