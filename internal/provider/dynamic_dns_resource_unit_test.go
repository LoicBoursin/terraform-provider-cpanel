package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"

	"terraform-provider-cpanel/internal/cpanel/ddns"
)

type dynamicDNSFakeClient struct {
	domain              *ddns.Domain
	getErr              error
	setDescriptionCalls int
	deleteCalls         int
	setDescriptionErr   error
	deleteErr           error
	deleteApplied       bool
}

func (c *dynamicDNSFakeClient) Get(
	_ context.Context,
	_ string,
) (*ddns.Domain, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	if c.domain == nil {
		return nil, nil
	}

	domain := *c.domain

	return &domain, nil
}

func (c *dynamicDNSFakeClient) Create(
	_ context.Context,
	_ string,
	_ string,
) (*ddns.CreatedDomain, error) {
	return nil, errors.New("unexpected Create call")
}

func (c *dynamicDNSFakeClient) SetDescription(
	_ context.Context,
	_ string,
	description string,
) error {
	c.setDescriptionCalls++
	if c.setDescriptionErr == nil && c.domain != nil {
		c.domain.Description = description
	}

	return c.setDescriptionErr
}

func (c *dynamicDNSFakeClient) Delete(
	_ context.Context,
	_ string,
) (bool, error) {
	c.deleteCalls++
	if c.deleteApplied {
		c.domain = nil
	}

	return c.deleteApplied, c.deleteErr
}

func TestDynamicDNSCreationMarkerIsUniqueAndOpaque(t *testing.T) {
	t.Parallel()

	first, err := newDynamicDNSCreationMarker()
	if err != nil {
		t.Fatalf("newDynamicDNSCreationMarker() error: %v", err)
	}
	second, err := newDynamicDNSCreationMarker()
	if err != nil {
		t.Fatalf("newDynamicDNSCreationMarker() error: %v", err)
	}
	if first == second {
		t.Fatal("creation markers must be unique")
	}
	for _, marker := range []string{first, second} {
		if !strings.HasPrefix(marker, "terraform-provider-cpanel:") ||
			len(marker) != len("terraform-provider-cpanel:")+32 {
			t.Fatalf("creation marker has unexpected format: %q", marker)
		}
	}
}

func TestReconcileDynamicDNSDescriptionRequiresStableIdentity(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		domain      *ddns.Domain
		wantApplied bool
		wantError   bool
	}{
		"attempted description": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "attempted",
			},
			wantApplied: true,
		},
		"previous description": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "previous",
			},
		},
		"concurrent description": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "concurrent",
			},
			wantError: true,
		},
		"replacement identity": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "replacement-id",
				Description: "attempted",
			},
			wantError: true,
		},
		"missing domain": {
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			resource := &dynamicDNSResource{
				client: &dynamicDNSFakeClient{domain: test.domain},
			}
			_, applied, err := resource.reconcileDynamicDNSDescription(
				t.Context(),
				"home.example.test",
				"expected-id",
				"previous",
				"attempted",
			)
			if (err != nil) != test.wantError {
				t.Fatalf("reconcile error = %v, wantError = %t", err, test.wantError)
			}
			if applied != test.wantApplied {
				t.Fatalf("reconcile applied = %t, want %t", applied, test.wantApplied)
			}
		})
	}
}

func TestRestoreDynamicDNSDescriptionPreservesConcurrentChanges(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		description string
		wantError   bool
		wantCalls   int
	}{
		"attempted state is restored": {
			description: "attempted",
			wantCalls:   1,
		},
		"already restored": {
			description: "previous",
		},
		"concurrent state is preserved": {
			description: "concurrent",
			wantError:   true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := &dynamicDNSFakeClient{
				domain: &ddns.Domain{
					Domain:      "home.example.test",
					ID:          "expected-id",
					Description: test.description,
				},
			}
			resource := &dynamicDNSResource{client: client}
			err := resource.restoreDynamicDNSDescription(
				t.Context(),
				"home.example.test",
				"expected-id",
				"attempted",
				"previous",
			)
			if (err != nil) != test.wantError {
				t.Fatalf("restore error = %v, wantError = %t", err, test.wantError)
			}
			if client.setDescriptionCalls != test.wantCalls {
				t.Fatalf(
					"SetDescription calls = %d, want %d",
					client.setDescriptionCalls,
					test.wantCalls,
				)
			}
		})
	}
}

func TestRollbackCreatedDynamicDNSRequiresAttributedDomain(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		domain        *ddns.Domain
		deleteApplied bool
		deleteErr     error
		wantError     bool
		wantCalls     int
	}{
		"marked domain is deleted": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "marker",
			},
			deleteApplied: true,
			wantCalls:     1,
		},
		"ambiguous applied deletion is accepted": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "marker",
			},
			deleteApplied: true,
			deleteErr:     errors.New("connection closed"),
			wantCalls:     1,
		},
		"replacement is preserved": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "replacement-id",
				Description: "marker",
			},
			wantError: true,
		},
		"concurrent description is preserved": {
			domain: &ddns.Domain{
				Domain:      "home.example.test",
				ID:          "expected-id",
				Description: "concurrent",
			},
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := &dynamicDNSFakeClient{
				domain:        test.domain,
				deleteApplied: test.deleteApplied,
				deleteErr:     test.deleteErr,
			}
			resource := &dynamicDNSResource{client: client}
			err := resource.rollbackCreatedDynamicDNS(
				t.Context(),
				"home.example.test",
				"expected-id",
				"marker",
			)
			if (err != nil) != test.wantError {
				t.Fatalf("rollback error = %v, wantError = %t", err, test.wantError)
			}
			if client.deleteCalls != test.wantCalls {
				t.Fatalf(
					"Delete calls = %d, want %d",
					client.deleteCalls,
					test.wantCalls,
				)
			}
		})
	}
}

func TestDynamicDNSDeleteRefusesDescriptionDrift(t *testing.T) {
	t.Parallel()

	expected := ddns.Domain{
		Domain:      "home.example.test",
		ID:          "expected-id",
		Description: "expected",
	}
	current := expected
	current.Description = "concurrent"
	client := &dynamicDNSFakeClient{domain: &current}
	resource := &dynamicDNSResource{client: client}

	response := runDynamicDNSDelete(t, resource, expected)

	if !response.Diagnostics.HasError() {
		t.Fatal("Delete() returned no error for description drift")
	}
	if client.deleteCalls != 0 {
		t.Fatalf("Delete calls = %d, want 0", client.deleteCalls)
	}
	if client.domain == nil || client.domain.Description != "concurrent" {
		t.Fatalf("Delete() changed concurrent domain: %#v", client.domain)
	}
}

func runDynamicDNSDelete(
	t *testing.T,
	resource *dynamicDNSResource,
	domain ddns.Domain,
) *frameworkresource.DeleteResponse {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewDynamicDNSResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	model := DynamicDNSResourceModel{}
	if diagnostics := applyDynamicDNSToResourceModel(
		t.Context(),
		&model,
		domain,
	); diagnostics.HasError() {
		t.Fatalf("applyDynamicDNSToResourceModel() diagnostics: %v", diagnostics)
	}
	state := tfsdk.State{
		Schema: schemaResponse.Schema,
	}
	if diagnostics := state.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.DeleteResponse{}
	resource.Delete(
		t.Context(),
		frameworkresource.DeleteRequest{State: state},
		response,
	)

	return response
}
