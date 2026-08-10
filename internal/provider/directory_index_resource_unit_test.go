package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/directoryindex"
)

func TestDirectoryIndexUpdateRefusesRemoteDrift(t *testing.T) {
	t.Parallel()

	const directory = "public_html/example"
	client := &fakeDirectoryIndexClient{
		current: &directoryindex.Index{
			Directory:         directory,
			Type:              directoryindex.IndexTypeStandard,
			AbsoluteDirectory: "/home/example/" + directory,
		},
	}
	resource := &directoryIndexResource{client: client}
	state := DirectoryIndexResourceModel{
		Directory:         types.StringValue(directory),
		Type:              types.StringValue(directoryindex.IndexTypeFancy),
		AbsoluteDirectory: types.StringValue("/home/example/" + directory),
	}
	plan := state
	plan.Type = types.StringValue(directoryindex.IndexTypeDisabled)

	response := runSingletonUpdate(
		t,
		NewDirectoryIndexResource(),
		state,
		plan,
		resource.Update,
	)
	assertUpdateDriftRefused(t, response.Diagnostics)
	if client.setCalls != 0 {
		t.Fatalf("setCalls = %d, want 0", client.setCalls)
	}
}

func TestDirectoryIndexResetReconcilesMutationResult(t *testing.T) {
	t.Parallel()

	const directory = "public_html/example"
	testCases := []struct {
		name        string
		setErr      error
		currentType string
		wantError   bool
	}{
		{
			name:        "ambiguous response after applied reset",
			setErr:      errors.New("response lost"),
			currentType: directoryindex.IndexTypeInherit,
		},
		{
			name:        "ambiguous response without reset",
			setErr:      errors.New("response lost"),
			currentType: directoryindex.IndexTypeFancy,
			wantError:   true,
		},
		{
			name:        "concurrent mode is preserved",
			currentType: directoryindex.IndexTypeStandard,
			wantError:   true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			client := &fakeDirectoryIndexClient{
				setErr: testCase.setErr,
				current: &directoryindex.Index{
					Directory: directory,
					Type:      testCase.currentType,
				},
			}
			resource := &directoryIndexResource{client: client}
			err := resource.resetDirectoryIndex(
				t.Context(),
				directory,
				directoryindex.IndexTypeFancy,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"resetDirectoryIndex() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if client.setCalls != 1 || client.getCalls != 1 {
				t.Fatalf(
					"calls = set:%d get:%d, want 1 each",
					client.setCalls,
					client.getCalls,
				)
			}
		})
	}
}

type fakeDirectoryIndexClient struct {
	current  *directoryindex.Index
	setErr   error
	setCalls int
	getCalls int
}

func (c *fakeDirectoryIndexClient) Get(
	_ context.Context,
	_ string,
) (*directoryindex.Index, error) {
	c.getCalls++

	return c.current, nil
}

func (c *fakeDirectoryIndexClient) Set(
	_ context.Context,
	_ string,
	_ string,
) (*directoryindex.Index, error) {
	c.setCalls++

	return c.current, c.setErr
}
