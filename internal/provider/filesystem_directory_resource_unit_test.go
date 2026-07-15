package provider

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

func TestFilesystemDirectoryResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewFilesystemDirectoryResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	pathAttribute, ok := response.Schema.Attributes["path"].(resourceschema.StringAttribute)
	if !ok ||
		!pathAttribute.Required ||
		len(pathAttribute.PlanModifiers) == 0 {
		t.Fatal("path must be a required replacement string")
	}
	for _, name := range []string{"absolute_path", "permissions", "owned"} {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsComputed() ||
			attribute.IsOptional() ||
			attribute.IsRequired() {
			t.Fatalf("%s must be computed-only", name)
		}
	}
}

func TestValidateFilesystemDirectoryPath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path      string
		wantError bool
	}{
		"valid":          {path: "public_html/assets"},
		"nested":         {path: "public_html/assets/images"},
		"root":           {path: "public_html", wantError: true},
		"outside root":   {path: "private/assets", wantError: true},
		"absolute":       {path: "/public_html/assets", wantError: true},
		"parent segment": {path: "public_html/../mail", wantError: true},
		"dot segment":    {path: "public_html/./assets", wantError: true},
		"double slash":   {path: "public_html//assets", wantError: true},
		"control":        {path: "public_html/assets\n", wantError: true},
		"backslash":      {path: `public_html\assets`, wantError: true},
		"too long": {
			path:      "public_html/" + strings.Repeat("a", 4090),
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateFilesystemDirectoryPath(test.path)
			if test.wantError && err == nil {
				t.Fatalf(
					"validateFilesystemDirectoryPath(%q) returned no error",
					test.path,
				)
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateFilesystemDirectoryPath(%q) error: %v",
					test.path,
					err,
				)
			}
		})
	}
}

func TestFilesystemDirectoryOwnershipMarkerRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	actual, err := parseFilesystemDirectoryMarker(content, directoryPath)
	if err != nil {
		t.Fatalf("parseFilesystemDirectoryMarker() error: %v", err)
	}
	if actual != token {
		t.Fatalf("marker token = %q, want %q", actual, token)
	}
	if _, err := parseFilesystemDirectoryMarker(
		content,
		"public_html/other",
	); err == nil {
		t.Fatal("parseFilesystemDirectoryMarker() accepted another path")
	}
}

func TestFilesystemDirectoryResolveOwnershipRecoversMarker(t *testing.T) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	client := newFakeFilesystemDirectoryClient()
	client.files[filesystemDirectoryMarkerPath(directoryPath)] = content
	resource := &filesystemDirectoryResource{client: client}

	actualToken, owned, recovered, diagnostics, err := resource.resolveOwnership(
		t.Context(),
		directoryPath,
		nil,
		true,
	)
	if diagnostics.HasError() {
		t.Fatalf("resolveOwnership() diagnostics: %v", diagnostics)
	}
	if err != nil {
		t.Fatalf("resolveOwnership() error: %v", err)
	}
	if actualToken != token || !owned || !recovered {
		t.Fatalf(
			"resolveOwnership() = %q, %t, %t; want %q, true, true",
			actualToken,
			owned,
			recovered,
			token,
		)
	}
}

func TestFilesystemDirectoryResolveOwnershipDoesNotAdoptMarker(
	t *testing.T,
) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	client := newFakeFilesystemDirectoryClient()
	client.files[filesystemDirectoryMarkerPath(directoryPath)] = content
	resource := &filesystemDirectoryResource{client: client}

	actualToken, owned, recovered, diagnostics, err := resource.resolveOwnership(
		t.Context(),
		directoryPath,
		nil,
		filesystemDirectoryMayOwn(types.BoolUnknown()),
	)
	if diagnostics.HasError() {
		t.Fatalf("resolveOwnership() diagnostics: %v", diagnostics)
	}
	if err != nil {
		t.Fatalf("resolveOwnership() error: %v", err)
	}
	if actualToken != "" || owned || recovered {
		t.Fatalf(
			"resolveOwnership() = %q, %t, %t; want empty, false, false",
			actualToken,
			owned,
			recovered,
		)
	}
}

func TestFilesystemDirectoryMayOwnRequiresKnownOwnedState(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		owned    types.Bool
		expected bool
	}{
		"null": {
			owned: types.BoolNull(),
		},
		"unknown": {
			owned: types.BoolUnknown(),
		},
		"false": {
			owned: types.BoolValue(false),
		},
		"true": {
			owned:    types.BoolValue(true),
			expected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if actual := filesystemDirectoryMayOwn(test.owned); actual != test.expected {
				t.Fatalf(
					"filesystemDirectoryMayOwn() = %t, want %t",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestFilesystemDirectoryDeleteOwnedDirectory(t *testing.T) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	client := newFakeFilesystemDirectoryClient()
	client.files[filesystemDirectoryMarkerPath(directoryPath)] = content
	resource := &filesystemDirectoryResource{client: client}

	if err := resource.deleteOwnedDirectory(
		t.Context(),
		directoryPath,
		token,
	); err != nil {
		t.Fatalf("deleteOwnedDirectory() error: %v", err)
	}
	if client.directory != nil {
		t.Fatal("directory still exists after deleteOwnedDirectory()")
	}
	if len(client.deleteCalls) != 2 ||
		client.deleteCalls[0] != filesystemDirectoryMarkerPath(directoryPath) ||
		client.deleteCalls[1] != directoryPath {
		t.Fatalf(
			"delete calls = %#v, want marker then directory",
			client.deleteCalls,
		)
	}
}

func TestFilesystemDirectoryDeleteRefusesExtraEntries(t *testing.T) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	client := newFakeFilesystemDirectoryClient()
	client.files[filesystemDirectoryMarkerPath(directoryPath)] = content
	client.files[path.Join(directoryPath, "user.txt")] = "preserve"
	resource := &filesystemDirectoryResource{client: client}

	err = resource.deleteOwnedDirectory(t.Context(), directoryPath, token)
	if err == nil {
		t.Fatal("deleteOwnedDirectory() returned no error")
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", client.deleteCalls)
	}
	if client.directory == nil ||
		client.files[path.Join(directoryPath, "user.txt")] != "preserve" {
		t.Fatal("deleteOwnedDirectory() changed an externally populated directory")
	}
}

func TestFilesystemDirectoryRollbackRequiresOwnershipMarker(t *testing.T) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	client := newFakeFilesystemDirectoryClient()
	resource := &filesystemDirectoryResource{client: client}

	if err := resource.rollbackCreatedDirectory(
		t.Context(),
		directoryPath,
		token,
	); err == nil {
		t.Fatal("rollbackCreatedDirectory() returned no error without ownership marker")
	}
	if client.directory == nil {
		t.Fatal("rollbackCreatedDirectory() deleted an unattributed directory")
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", client.deleteCalls)
	}
}

func TestFilesystemDirectoryDeleteRestoresMarkerAfterDirectoryFailure(
	t *testing.T,
) {
	t.Parallel()

	const (
		directoryPath = "public_html/assets"
		token         = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow
	)
	content, err := filesystemDirectoryMarkerContent(directoryPath, token)
	if err != nil {
		t.Fatalf("filesystemDirectoryMarkerContent() error: %v", err)
	}
	client := newFakeFilesystemDirectoryClient()
	client.files[filesystemDirectoryMarkerPath(directoryPath)] = content
	client.deleteErrors[directoryPath] = errors.New("directory deletion failed")
	resource := &filesystemDirectoryResource{client: client}

	err = resource.deleteOwnedDirectory(t.Context(), directoryPath, token)
	if err == nil {
		t.Fatal("deleteOwnedDirectory() returned no error")
	}
	if client.directory == nil {
		t.Fatal("directory disappeared despite the simulated delete failure")
	}
	if actual := client.files[filesystemDirectoryMarkerPath(directoryPath)]; actual != content {
		t.Fatalf("restored marker = %q, want %q", actual, content)
	}
}

func TestFilesystemDirectoryModelMapping(t *testing.T) {
	t.Parallel()

	directory := fileman.Directory{Entry: fileman.Entry{
		Path:         "public_html/assets",
		AbsolutePath: "/home/example/public_html/assets",
		Type:         fileman.EntryTypeDirectory,
		Permissions:  "0755",
	}}

	resourceModel := FilesystemDirectoryResourceModel{}
	applyFilesystemDirectoryToResourceModel(
		&resourceModel,
		directory,
		true,
	)
	if resourceModel.Path.ValueString() != directory.Entry.Path ||
		resourceModel.AbsolutePath.ValueString() != directory.Entry.AbsolutePath ||
		resourceModel.Permissions.ValueString() != directory.Entry.Permissions ||
		!resourceModel.Owned.ValueBool() {
		t.Fatalf("resource model = %#v", resourceModel)
	}

	dataSourceModel := filesystemDirectoryToDataSourceModel(directory)
	if dataSourceModel.Path.ValueString() != directory.Entry.Path ||
		dataSourceModel.AbsolutePath.ValueString() != directory.Entry.AbsolutePath ||
		dataSourceModel.Permissions.ValueString() != directory.Entry.Permissions {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}

type fakeFilesystemDirectoryClient struct {
	directory    *fileman.Directory
	files        map[string]string
	deleteErrors map[string]error
	deleteCalls  []string
}

func newFakeFilesystemDirectoryClient() *fakeFilesystemDirectoryClient {
	const directoryPath = "public_html/assets"

	return &fakeFilesystemDirectoryClient{
		directory: &fileman.Directory{Entry: fileman.Entry{
			Path:         directoryPath,
			AbsolutePath: path.Join("/home/example", directoryPath),
			Type:         fileman.EntryTypeDirectory,
			Permissions:  "0755",
		}},
		files:        make(map[string]string),
		deleteErrors: make(map[string]error),
	}
}

func (c *fakeFilesystemDirectoryClient) LockMutations() func() {
	return func() {}
}

func (c *fakeFilesystemDirectoryClient) GetDirectory(
	_ context.Context,
	directoryPath string,
) (*fileman.Directory, error) {
	if c.directory == nil || c.directory.Entry.Path != directoryPath {
		return nil, nil
	}
	directoryCopy := *c.directory

	return &directoryCopy, nil
}

func (c *fakeFilesystemDirectoryClient) ListDirectory(
	_ context.Context,
	directoryPath string,
) ([]fileman.Entry, error) {
	if c.directory == nil || c.directory.Entry.Path != directoryPath {
		return nil, nil
	}

	entries := make([]fileman.Entry, 0, len(c.files))
	for filePath := range c.files {
		if path.Dir(filePath) != directoryPath {
			continue
		}
		entries = append(entries, fileman.Entry{
			Path:         filePath,
			AbsolutePath: path.Join("/home/example", filePath),
			Type:         fileman.EntryTypeFile,
			Permissions:  "0644",
		})
	}

	return entries, nil
}

func (c *fakeFilesystemDirectoryClient) CreateDirectory(
	_ context.Context,
	directoryPath string,
) (*fileman.Directory, error) {
	if c.directory != nil {
		return nil, fmt.Errorf("path already exists")
	}
	c.directory = &fileman.Directory{Entry: fileman.Entry{
		Path:         directoryPath,
		AbsolutePath: path.Join("/home/example", directoryPath),
		Type:         fileman.EntryTypeDirectory,
		Permissions:  "0755",
	}}
	directoryCopy := *c.directory

	return &directoryCopy, nil
}

func (c *fakeFilesystemDirectoryClient) GetTextFile(
	_ context.Context,
	filePath string,
) (*fileman.TextFile, error) {
	content, exists := c.files[filePath]
	if !exists {
		return nil, nil
	}

	return &fileman.TextFile{
		Entry: fileman.Entry{
			Path:         filePath,
			AbsolutePath: path.Join("/home/example", filePath),
			Type:         fileman.EntryTypeFile,
			Permissions:  "0644",
		},
		Content: content,
	}, nil
}

func (c *fakeFilesystemDirectoryClient) SaveTextFile(
	_ context.Context,
	filePath string,
	content string,
) (*fileman.TextFile, error) {
	c.files[filePath] = content

	return c.GetTextFile(context.Background(), filePath)
}

func (c *fakeFilesystemDirectoryClient) DeletePath(
	_ context.Context,
	managedPath string,
) error {
	c.deleteCalls = append(c.deleteCalls, managedPath)
	if err := c.deleteErrors[managedPath]; err != nil {
		return err
	}
	if c.directory != nil && c.directory.Entry.Path == managedPath {
		if len(c.files) != 0 {
			return fmt.Errorf("directory is not empty")
		}
		c.directory = nil

		return nil
	}
	delete(c.files, managedPath)

	return nil
}
