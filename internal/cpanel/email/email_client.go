package email

import (
	"context"
	"net/http"
	"sync"

	"terraform-provider-cpanel/internal/cpanel"
)

type Client struct {
	*cpanel.Client

	filterLocksMu sync.Mutex
	filterLocks   map[string]*filterAccountLock
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:      client,
		filterLocks: make(map[string]*filterAccountLock),
	}
}

type filterAccountLock struct {
	mutex      sync.Mutex
	references int
}

func (c *Client) LockFilterAccount(account string) func() {
	c.filterLocksMu.Lock()
	if c.filterLocks == nil {
		c.filterLocks = make(map[string]*filterAccountLock)
	}
	lock := c.filterLocks[account]
	if lock == nil {
		lock = &filterAccountLock{}
		c.filterLocks[account] = lock
	}
	lock.references++
	c.filterLocksMu.Unlock()

	lock.mutex.Lock()

	var once sync.Once

	return func() {
		once.Do(func() {
			lock.mutex.Unlock()

			c.filterLocksMu.Lock()
			lock.references--
			if lock.references == 0 {
				delete(c.filterLocks, account)
			}
			c.filterLocksMu.Unlock()
		})
	}
}

func (c *Client) executeReadOperation(
	ctx context.Context,
	function string,
	parameters map[string]string,
	output any,
) error {
	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodGet,
		cpanel.ModuleEmail,
		function,
		parameters,
		output,
	)
}

func (c *Client) executeMutation(
	ctx context.Context,
	function string,
	parameters map[string]string,
	output any,
) error {
	return c.ExecuteUAPIOperation(
		ctx,
		http.MethodPost,
		cpanel.ModuleEmail,
		function,
		parameters,
		output,
	)
}
