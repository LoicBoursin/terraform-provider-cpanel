package provider

import (
	"context"
	"errors"
	"testing"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailFilterApplyRemoteHandlesAmbiguousMutations(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		storeError     error
		applyStore     bool
		statusError    error
		applyStatus    bool
		wantError      bool
		wantStatusCall bool
	}{
		{
			name:           "ordinary success",
			applyStore:     true,
			applyStatus:    true,
			wantStatusCall: true,
		},
		{
			name:           "ambiguous store applied",
			storeError:     errors.New("store response lost"),
			applyStore:     true,
			applyStatus:    true,
			wantStatusCall: true,
		},
		{
			name:        "ambiguous store not applied",
			storeError:  errors.New("store failed"),
			applyStore:  false,
			applyStatus: true,
			wantError:   true,
		},
		{
			name:           "ambiguous status applied",
			applyStore:     true,
			statusError:    errors.New("status response lost"),
			applyStatus:    true,
			wantStatusCall: true,
		},
		{
			name:           "ambiguous status not applied",
			applyStore:     true,
			statusError:    errors.New("status failed"),
			applyStatus:    false,
			wantError:      true,
			wantStatusCall: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desired := testEmailFilterDefinition()
			desired.Enabled = false
			client := newFakeEmailFilterClient()
			client.storeError = testCase.storeError
			client.applyStore = testCase.applyStore
			client.statusError = testCase.statusError
			client.applyStatus = testCase.applyStatus
			resource := emailFilterResource{client: client}

			actual, err := resource.applyRemoteFilter(
				t.Context(),
				"",
				desired,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"applyRemoteFilter() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if testCase.wantStatusCall != (client.statusCalls == 1) {
				t.Fatalf(
					"status calls = %d, wantStatusCall %t",
					client.statusCalls,
					testCase.wantStatusCall,
				)
			}
			if testCase.wantError {
				return
			}
			if actual == nil || !emailFiltersEqual(*actual, desired) {
				t.Fatalf(
					"applyRemoteFilter() = %#v, want %#v",
					actual,
					desired,
				)
			}
		})
	}
}

func TestEmailFilterRollbackCreatedRequiresExactState(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		current         *cpanelmail.Filter
		wantError       bool
		wantDeleteCalls int
	}{
		{
			name:            "exact state",
			current:         filterPointer(testEmailFilterDefinition()),
			wantDeleteCalls: 1,
		},
		{
			name: "status changed",
			current: filterPointer(func() cpanelmail.Filter {
				filter := testEmailFilterDefinition()
				filter.Enabled = false

				return filter
			}()),
			wantError: true,
		},
		{
			name: "definition changed",
			current: filterPointer(func() cpanelmail.Filter {
				filter := testEmailFilterDefinition()
				filter.Rules[0].Value = "external change"

				return filter
			}()),
			wantError: true,
		},
		{name: "already absent"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			desired := testEmailFilterDefinition()
			client := newFakeEmailFilterClient()
			if testCase.current != nil {
				client.put(*testCase.current)
			}
			resource := emailFilterResource{client: client}

			err := resource.rollbackCreatedFilter(t.Context(), desired)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"rollbackCreatedFilter() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if client.deleteCalls != testCase.wantDeleteCalls {
				t.Fatalf(
					"delete calls = %d, want %d",
					client.deleteCalls,
					testCase.wantDeleteCalls,
				)
			}
		})
	}
}

func TestEmailFilterRollbackUpdatedRequiresExactOwnership(t *testing.T) {
	t.Parallel()

	previous := testEmailFilterDefinition()
	desired := cloneEmailFilter(previous)
	desired.Name = "Terraform filter renamed"
	desired.Enabled = false
	desired.Rules[0].Value = "Updated by Terraform"

	t.Run("restores exact renamed state", func(t *testing.T) {
		client := newFakeEmailFilterClient()
		client.put(desired)
		resource := emailFilterResource{client: client}

		if err := resource.rollbackUpdatedFilter(
			t.Context(),
			previous,
			desired,
		); err != nil {
			t.Fatalf("rollbackUpdatedFilter() error: %v", err)
		}
		assertFakeEmailFilter(t, client, previous)
		if filter := client.get(desired.Account, desired.Name); filter != nil {
			t.Fatalf("renamed filter still exists: %#v", filter)
		}
	})

	t.Run("accepts already restored state", func(t *testing.T) {
		client := newFakeEmailFilterClient()
		client.put(previous)
		resource := emailFilterResource{client: client}

		if err := resource.rollbackUpdatedFilter(
			t.Context(),
			previous,
			desired,
		); err != nil {
			t.Fatalf("rollbackUpdatedFilter() error: %v", err)
		}
		if client.storeCalls != 0 || client.statusCalls != 0 {
			t.Fatalf(
				"rollback mutated an already restored filter: stores=%d statuses=%d",
				client.storeCalls,
				client.statusCalls,
			)
		}
	})

	t.Run("refuses status mismatch", func(t *testing.T) {
		client := newFakeEmailFilterClient()
		current := cloneEmailFilter(desired)
		current.Enabled = !desired.Enabled
		client.put(current)
		resource := emailFilterResource{client: client}

		if err := resource.rollbackUpdatedFilter(
			t.Context(),
			previous,
			desired,
		); err == nil {
			t.Fatal("rollbackUpdatedFilter() returned no error")
		}
		if client.storeCalls != 0 {
			t.Fatalf("store calls = %d, want 0", client.storeCalls)
		}
	})

	t.Run("refuses two live identities", func(t *testing.T) {
		client := newFakeEmailFilterClient()
		client.put(previous)
		client.put(desired)
		resource := emailFilterResource{client: client}

		if err := resource.rollbackUpdatedFilter(
			t.Context(),
			previous,
			desired,
		); err == nil {
			t.Fatal("rollbackUpdatedFilter() returned no error")
		}
		if client.storeCalls != 0 {
			t.Fatalf("store calls = %d, want 0", client.storeCalls)
		}
	})
}

type fakeEmailFilterClient struct {
	filters map[string]cpanelmail.Filter

	storeError  error
	statusError error
	deleteError error
	applyStore  bool
	applyStatus bool
	applyDelete bool

	storeCalls  int
	statusCalls int
	deleteCalls int
}

func newFakeEmailFilterClient() *fakeEmailFilterClient {
	return &fakeEmailFilterClient{
		filters:     make(map[string]cpanelmail.Filter),
		applyStore:  true,
		applyStatus: true,
		applyDelete: true,
	}
}

func (c *fakeEmailFilterClient) DeleteFilter(
	_ context.Context,
	account string,
	name string,
) error {
	c.deleteCalls++
	if c.applyDelete {
		delete(c.filters, emailFilterFakeKey(account, name))
	}

	return c.deleteError
}

func (c *fakeEmailFilterClient) GetAccount(
	_ context.Context,
	user string,
	domain string,
) (*cpanelmail.Account, error) {
	return &cpanelmail.Account{
		Email:  user + "@" + domain,
		User:   user,
		Domain: domain,
	}, nil
}

func (c *fakeEmailFilterClient) GetFilter(
	_ context.Context,
	account string,
	name string,
) (*cpanelmail.Filter, error) {
	return c.get(account, name), nil
}

func (c *fakeEmailFilterClient) LockFilterAccount(string) func() {
	return func() {}
}

func (c *fakeEmailFilterClient) SetFilterEnabled(
	_ context.Context,
	account string,
	name string,
	enabled bool,
) error {
	c.statusCalls++
	if c.applyStatus {
		key := emailFilterFakeKey(account, name)
		if filter, ok := c.filters[key]; ok {
			filter.Enabled = enabled
			c.filters[key] = filter
		}
	}

	return c.statusError
}

func (c *fakeEmailFilterClient) StoreFilter(
	_ context.Context,
	account string,
	oldName string,
	desired cpanelmail.Filter,
) error {
	c.storeCalls++
	if c.applyStore {
		enabled := true
		if current := c.get(account, oldName); current != nil {
			enabled = current.Enabled
		}
		if oldName != "" {
			delete(c.filters, emailFilterFakeKey(account, oldName))
		}

		stored := cloneEmailFilter(desired)
		stored.Account = account
		stored.Enabled = enabled
		c.put(stored)
	}

	return c.storeError
}

func (c *fakeEmailFilterClient) get(
	account string,
	name string,
) *cpanelmail.Filter {
	filter, ok := c.filters[emailFilterFakeKey(account, name)]
	if !ok {
		return nil
	}

	cloned := cloneEmailFilter(filter)

	return &cloned
}

func (c *fakeEmailFilterClient) put(filter cpanelmail.Filter) {
	c.filters[emailFilterFakeKey(filter.Account, filter.Name)] = cloneEmailFilter(
		filter,
	)
}

func assertFakeEmailFilter(
	t *testing.T,
	client *fakeEmailFilterClient,
	expected cpanelmail.Filter,
) {
	t.Helper()

	actual := client.get(expected.Account, expected.Name)
	if actual == nil || !emailFiltersEqual(*actual, expected) {
		t.Fatalf("email filter = %#v, want %#v", actual, expected)
	}
}

func cloneEmailFilter(filter cpanelmail.Filter) cpanelmail.Filter {
	filter.Rules = append([]cpanelmail.FilterRule(nil), filter.Rules...)
	filter.Actions = append([]cpanelmail.FilterAction(nil), filter.Actions...)

	return filter
}

func emailFilterFakeKey(account, name string) string {
	return account + "\x00" + name
}

func filterPointer(filter cpanelmail.Filter) *cpanelmail.Filter {
	return &filter
}
