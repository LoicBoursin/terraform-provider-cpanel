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

	mailingListLocksMu sync.Mutex
	mailingListLocks   map[string]*mailingListAddressLock

	routingLocksMu sync.Mutex
	routingLocks   map[string]*routingDomainLock
}

func NewClient(client *cpanel.Client) *Client {
	return &Client{
		Client:           client,
		filterLocks:      make(map[string]*filterAccountLock),
		mailingListLocks: make(map[string]*mailingListAddressLock),
		routingLocks:     make(map[string]*routingDomainLock),
	}
}

type filterAccountLock struct {
	mutex      sync.Mutex
	references int
}

type mailingListAddressLock struct {
	mutex      sync.Mutex
	references int
}

type routingDomainLock struct {
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

func (c *Client) LockAccountFilters() func() {
	return c.LockFilterAccount("\x00account-level")
}

func (c *Client) LockMailingList(address string) func() {
	c.mailingListLocksMu.Lock()
	if c.mailingListLocks == nil {
		c.mailingListLocks = make(map[string]*mailingListAddressLock)
	}
	lock := c.mailingListLocks[address]
	if lock == nil {
		lock = &mailingListAddressLock{}
		c.mailingListLocks[address] = lock
	}
	lock.references++
	c.mailingListLocksMu.Unlock()

	lock.mutex.Lock()

	var once sync.Once

	return func() {
		once.Do(func() {
			lock.mutex.Unlock()

			c.mailingListLocksMu.Lock()
			lock.references--
			if lock.references == 0 {
				delete(c.mailingListLocks, address)
			}
			c.mailingListLocksMu.Unlock()
		})
	}
}

func (c *Client) LockRoutingDomain(domain string) func() {
	c.routingLocksMu.Lock()
	if c.routingLocks == nil {
		c.routingLocks = make(map[string]*routingDomainLock)
	}
	lock := c.routingLocks[domain]
	if lock == nil {
		lock = &routingDomainLock{}
		c.routingLocks[domain] = lock
	}
	lock.references++
	c.routingLocksMu.Unlock()

	lock.mutex.Lock()

	var once sync.Once

	return func() {
		once.Do(func() {
			lock.mutex.Unlock()

			c.routingLocksMu.Lock()
			lock.references--
			if lock.references == 0 {
				delete(c.routingLocks, domain)
			}
			c.routingLocksMu.Unlock()
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
