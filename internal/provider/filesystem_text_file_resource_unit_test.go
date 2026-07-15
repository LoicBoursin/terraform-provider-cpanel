package provider

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	frameworkdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	datasourceschema "github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/fileman"
)

const testFilesystemTextFileOwnershipToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" // gitleaks:allow

func TestFilesystemTextFileResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewFilesystemTextFileResource().Schema(
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
	contentAttribute, ok := response.Schema.Attributes["content"].(resourceschema.StringAttribute)
	if !ok || !contentAttribute.Required || !contentAttribute.Sensitive {
		t.Fatal("content must be a required sensitive string")
	}
	for _, name := range []string{
		"absolute_path",
		"permissions",
		"size_bytes",
		"content_sha256",
		"owned",
		"content_matches_ownership_marker",
	} {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsComputed() ||
			attribute.IsOptional() ||
			attribute.IsRequired() {
			t.Fatalf("%s must be computed-only", name)
		}
	}
}

func TestFilesystemTextFileDataSourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkdatasource.SchemaResponse{}
	NewFilesystemTextFileDataSource().Schema(
		t.Context(),
		frameworkdatasource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	pathAttribute, ok := response.Schema.Attributes["path"].(datasourceschema.StringAttribute)
	if !ok || !pathAttribute.Required {
		t.Fatal("path must be a required string")
	}
	contentAttribute, ok := response.Schema.Attributes["content"].(datasourceschema.StringAttribute)
	if !ok || !contentAttribute.Computed || !contentAttribute.Sensitive {
		t.Fatal("content must be a computed sensitive string")
	}
	for _, name := range []string{
		"absolute_path",
		"permissions",
		"size_bytes",
		"content_sha256",
	} {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsComputed() ||
			attribute.IsOptional() ||
			attribute.IsRequired() {
			t.Fatalf("%s must be computed-only", name)
		}
	}
}

func TestValidateFilesystemTextFilePath(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		path      string
		wantError bool
	}{
		"valid":                     {path: "public_html/robots.txt"},
		"nested":                    {path: "public_html/assets/site.txt"},
		"unicode":                   {path: "public_html/caf\u00e9.txt"},
		"root":                      {path: "public_html", wantError: true},
		"outside root":              {path: "private/site.txt", wantError: true},
		"absolute":                  {path: "/public_html/site.txt", wantError: true},
		"parent segment":            {path: "public_html/../mail/site.txt", wantError: true},
		"dot segment":               {path: "public_html/./site.txt", wantError: true},
		"double slash":              {path: "public_html//site.txt", wantError: true},
		"control":                   {path: "public_html/site\n.txt", wantError: true},
		"backslash":                 {path: `public_html\site.txt`, wantError: true},
		"comma":                     {path: "public_html/site,other.txt", wantError: true},
		"cgi-bin":                   {path: "public_html/cgi-bin/site.txt", wantError: true},
		"directory marker":          {path: "public_html/" + filesystemDirectoryMarkerName, wantError: true},
		"text file marker":          {path: "public_html/" + filesystemTextFileMarkerPrefix + "abc", wantError: true},
		"segment exceeds 255 bytes": {path: "public_html/" + strings.Repeat("a", 256), wantError: true},
		"path exceeds 4096 bytes":   {path: "public_html/" + strings.Repeat("a/", 2045) + "a", wantError: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateFilesystemTextFilePath(test.path)
			if test.wantError && err == nil {
				t.Fatalf(
					"validateFilesystemTextFilePath(%q) returned no error",
					test.path,
				)
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateFilesystemTextFilePath(%q) error: %v",
					test.path,
					err,
				)
			}
		})
	}
}

func TestValidateFilesystemTextFileContent(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		content   string
		wantError bool
	}{
		"empty":         {},
		"unicode":       {content: "caf\u00e9\n"},
		"maximum bytes": {content: strings.Repeat("a", filesystemTextFileMaximumContentBytes)},
		"invalid UTF-8": {content: string([]byte{0xff}), wantError: true},
		"null byte":     {content: "a\x00b", wantError: true},
		"too large": {
			content:   strings.Repeat("a", filesystemTextFileMaximumContentBytes+1),
			wantError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := validateFilesystemTextFileContent(test.content)
			if test.wantError && err == nil {
				t.Fatal("validateFilesystemTextFileContent() returned no error")
			}
			if !test.wantError && err != nil {
				t.Fatalf(
					"validateFilesystemTextFileContent() error: %v",
					err,
				)
			}
		})
	}
}

func TestFilesystemTextFileOwnershipMarkerRoundTrip(t *testing.T) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "caf\u00e9\n"
	)
	marker, err := newFilesystemTextFileMarker(
		filePath,
		testFilesystemTextFileOwnershipToken,
		content,
	)
	if err != nil {
		t.Fatalf("newFilesystemTextFileMarker() error: %v", err)
	}
	markerContent, err := filesystemTextFileMarkerContent(marker)
	if err != nil {
		t.Fatalf("filesystemTextFileMarkerContent() error: %v", err)
	}
	parsed, err := parseFilesystemTextFileMarker(markerContent, filePath)
	if err != nil {
		t.Fatalf("parseFilesystemTextFileMarker() error: %v", err)
	}
	if parsed != marker {
		t.Fatalf("parsed marker = %#v, want %#v", parsed, marker)
	}
	if parsed.SizeBytes != int64(len([]byte(content))) ||
		parsed.ContentSHA256 != filesystemTextFileContentSHA256(content) {
		t.Fatalf("marker content identity = %#v", parsed)
	}

	tests := map[string]string{
		"other path": strings.Replace(
			markerContent,
			filePath,
			"public_html/other.txt",
			1,
		),
		"unknown field":           strings.TrimSuffix(markerContent, "}") + `,"extra":true}`,
		"trailing newline":        markerContent + "\n",
		"noncanonical whitespace": strings.Replace(markerContent, `{"provider"`, `{ "provider"`, 1),
	}
	for name, invalidContent := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := parseFilesystemTextFileMarker(
				invalidContent,
				filePath,
			); err == nil {
				t.Fatal("parseFilesystemTextFileMarker() accepted invalid content")
			}
		})
	}
}

func TestFilesystemTextFileOwnershipMarkerSizeLimit(t *testing.T) {
	t.Parallel()

	buildPath := func(segmentCount int, segment string) string {
		segments := make([]string, 0, segmentCount+2)
		segments = append(segments, "public_html")
		for range segmentCount {
			segments = append(segments, segment)
		}
		segments = append(segments, "site.txt")

		return strings.Join(segments, "/")
	}

	acceptedPath := buildPath(15, strings.Repeat("a", 250))
	if err := validateFilesystemTextFilePath(acceptedPath); err != nil {
		t.Fatalf("accepted path validation error: %v", err)
	}
	acceptedMarker, err := newFilesystemTextFileMarker(
		acceptedPath,
		testFilesystemTextFileOwnershipToken,
		"",
	)
	if err != nil {
		t.Fatalf("newFilesystemTextFileMarker() error: %v", err)
	}
	if _, err := filesystemTextFileMarkerContent(
		acceptedMarker,
	); err != nil {
		t.Fatalf("filesystemTextFileMarkerContent() error: %v", err)
	}

	oversizedPath := buildPath(16, strings.Repeat("a", 250))
	if err := validateFilesystemTextFilePath(oversizedPath); err != nil {
		t.Fatalf("oversized marker path validation error: %v", err)
	}
	oversizedMarker, err := newFilesystemTextFileMarker(
		oversizedPath,
		testFilesystemTextFileOwnershipToken,
		"",
	)
	if err != nil {
		t.Fatalf("newFilesystemTextFileMarker() error: %v", err)
	}
	if _, err := filesystemTextFileMarkerContent(
		oversizedMarker,
	); err == nil {
		t.Fatal("filesystemTextFileMarkerContent() accepted an oversized marker")
	}

	escapedPath := buildPath(10, strings.Repeat("<", 250))
	if err := validateFilesystemTextFilePath(escapedPath); err != nil {
		t.Fatalf("escaped path validation error: %v", err)
	}
	escapedMarker, err := newFilesystemTextFileMarker(
		escapedPath,
		testFilesystemTextFileOwnershipToken,
		"",
	)
	if err != nil {
		t.Fatalf("newFilesystemTextFileMarker() error: %v", err)
	}
	if _, err := filesystemTextFileMarkerContent(
		escapedMarker,
	); err == nil {
		t.Fatal("filesystemTextFileMarkerContent() accepted escaped oversized marker")
	}
}

func TestFilesystemTextFileResolveOwnership(t *testing.T) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "managed\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, content)
	client.setTestOwnershipMarker(t)
	resource := &filesystemTextFileResource{client: client}

	ownership, diagnostics, err := resource.resolveOwnership(
		t.Context(),
		filePath,
		nil,
		true,
	)
	if diagnostics.HasError() {
		t.Fatalf("resolveOwnership() diagnostics: %v", diagnostics)
	}
	if err != nil {
		t.Fatalf("resolveOwnership() error: %v", err)
	}
	if !ownership.Owned ||
		!ownership.Recovered ||
		ownership.Token != testFilesystemTextFileOwnershipToken ||
		!filesystemTextFileMarkerMatches(
			client.files[filePath],
			ownership.Marker,
		) {
		t.Fatalf("resolveOwnership() = %#v", ownership)
	}

	unowned, diagnostics, err := resource.resolveOwnership(
		t.Context(),
		filePath,
		nil,
		filesystemTextFileMayOwn(types.BoolUnknown()),
	)
	if diagnostics.HasError() {
		t.Fatalf("resolveOwnership(false) diagnostics: %v", diagnostics)
	}
	if err != nil {
		t.Fatalf("resolveOwnership(false) error: %v", err)
	}
	if unowned.Owned || unowned.Recovered || unowned.Token != "" {
		t.Fatalf("resolveOwnership(false) = %#v", unowned)
	}
}

func TestFilesystemTextFileMayOwnRequiresKnownOwnedState(t *testing.T) {
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

			if actual := filesystemTextFileMayOwn(test.owned); actual != test.expected {
				t.Fatalf(
					"filesystemTextFileMayOwn() = %t, want %t",
					actual,
					test.expected,
				)
			}
		})
	}
}

func TestFilesystemTextFileResolveOwnershipRejectsPrivateTokenMismatch(
	t *testing.T,
) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "managed\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, content)
	client.setTestOwnershipMarker(t)
	privateState := newFakeFilesystemTextFilePrivateState()
	otherToken := strings.Repeat("a", filesystemTextFileOwnershipTokenHexSize)
	diagnostics := writeFilesystemTextFileOwnershipToken(
		t.Context(),
		privateState,
		otherToken,
	)
	if diagnostics.HasError() {
		t.Fatalf("writeFilesystemTextFileOwnershipToken() diagnostics: %v", diagnostics)
	}

	resource := &filesystemTextFileResource{client: client}
	_, diagnostics, err := resource.resolveOwnership(
		t.Context(),
		filePath,
		privateState,
		true,
	)
	if diagnostics.HasError() {
		t.Fatalf("resolveOwnership() diagnostics: %v", diagnostics)
	}
	if err == nil {
		t.Fatal("resolveOwnership() accepted a mismatched private token")
	}
}

func TestFilesystemTextFileOwnershipMatchPlanModifier(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		owned     bool
		matches   bool
		wantValue bool
	}{
		"owned and matching": {
			owned:     true,
			matches:   true,
			wantValue: true,
		},
		"owned and stale": {
			owned:     true,
			matches:   false,
			wantValue: true,
		},
		"unowned import": {
			owned:     false,
			matches:   false,
			wantValue: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := testFilesystemTextFileState(
				t,
				"content\n",
				test.owned,
				test.matches,
			)
			request := planmodifier.BoolRequest{
				State:      state,
				Plan:       tfsdk.Plan(state),
				StateValue: types.BoolValue(test.matches),
				PlanValue:  types.BoolUnknown(),
			}
			response := &planmodifier.BoolResponse{
				PlanValue: request.PlanValue,
			}
			filesystemTextFileOwnershipMatchPlanModifier{}.PlanModifyBool(
				t.Context(),
				request,
				response,
			)
			if response.Diagnostics.HasError() {
				t.Fatalf(
					"PlanModifyBool() diagnostics: %v",
					response.Diagnostics,
				)
			}
			if response.PlanValue.IsNull() ||
				response.PlanValue.IsUnknown() ||
				response.PlanValue.ValueBool() != test.wantValue {
				t.Fatalf(
					"PlanModifyBool() value = %v, want %t",
					response.PlanValue,
					test.wantValue,
				)
			}
		})
	}
}

func TestReadFilesystemTextFileRejectsOversizedEntryBeforeContent(t *testing.T) {
	t.Parallel()

	const filePath = "public_html/site.txt"
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, "small")
	client.sizeOverrides[filePath] = filesystemTextFileMaximumContentBytes + 1

	if _, err := readFilesystemTextFile(
		t.Context(),
		client,
		filePath,
	); err == nil {
		t.Fatal("readFilesystemTextFile() accepted an oversized entry")
	}
	if client.getTextFileCalls != 0 {
		t.Fatalf(
			"GetTextFile() calls = %d; want 0",
			client.getTextFileCalls,
		)
	}
}

func TestReadFilesystemTextFileRejectsReportedSizeMismatch(t *testing.T) {
	t.Parallel()

	const filePath = "public_html/site.txt"
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, "content")
	client.sizeOverrides[filePath] = 2

	if _, err := readFilesystemTextFile(
		t.Context(),
		client,
		filePath,
	); err == nil {
		t.Fatal("readFilesystemTextFile() accepted a reported size mismatch")
	}
}

func TestFilesystemTextFileRollbackDeletesOnlyExpectedContents(t *testing.T) {
	t.Parallel()

	const (
		filePath      = "public_html/site.txt"
		content       = "managed\n"
		changed       = "external\n"
		markerContent = "marker"
	)
	markerPath := filesystemTextFileMarkerPath(filePath)

	t.Run("exact", func(t *testing.T) {
		t.Parallel()

		client := newFakeFilesystemTextFileClient()
		client.setFile(filePath, content)
		client.setFile(markerPath, markerContent)
		resource := &filesystemTextFileResource{client: client}

		if err := resource.rollbackCreatedTextFile(
			t.Context(),
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		); err != nil {
			t.Fatalf("rollbackCreatedTextFile() error: %v", err)
		}
		if len(client.files) != 0 {
			t.Fatalf("remaining files = %#v", client.files)
		}
	})

	t.Run("changed target", func(t *testing.T) {
		t.Parallel()

		client := newFakeFilesystemTextFileClient()
		client.setFile(filePath, changed)
		client.setFile(markerPath, markerContent)
		resource := &filesystemTextFileResource{client: client}

		if err := resource.rollbackCreatedTextFile(
			t.Context(),
			filePath,
			content,
			markerPath,
			markerContent,
			true,
		); err == nil {
			t.Fatal("rollbackCreatedTextFile() returned no error")
		}
		if actual := client.files[filePath].Content; actual != changed {
			t.Fatalf("changed target content = %q, want %q", actual, changed)
		}
	})
}

func TestFilesystemTextFileCreatePreservesAmbiguousEmptyMarker(t *testing.T) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "managed\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.createErrors[filesystemTextFileMarkerPath(filePath)] = errors.New(
		"ambiguous mkfile response",
	)
	resource := &filesystemTextFileResource{
		client: client,
		generateOwnershipToken: func() (string, error) {
			return testFilesystemTextFileOwnershipToken, nil
		},
	}

	schemaResponse := &frameworkresource.SchemaResponse{}
	resource.Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics := plan.Set(t.Context(), &FilesystemTextFileResourceModel{
		Path:                          types.StringValue(filePath),
		Content:                       types.StringValue(content),
		AbsolutePath:                  types.StringUnknown(),
		Permissions:                   types.StringUnknown(),
		SizeBytes:                     types.Int64Unknown(),
		ContentSHA256:                 types.StringUnknown(),
		Owned:                         types.BoolUnknown(),
		ContentMatchesOwnershipMarker: types.BoolUnknown(),
	})
	if diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	resource.Create(
		t.Context(),
		frameworkresource.CreateRequest{Plan: plan},
		response,
	)
	if !response.Diagnostics.HasError() {
		t.Fatal("Create() returned no error for an ambiguous marker response")
	}
	if client.files[filePath].Content != content {
		t.Fatalf("target file = %#v, want preserved content", client.files[filePath])
	}
	markerPath := filesystemTextFileMarkerPath(filePath)
	if client.files[markerPath].Content != "" {
		t.Fatalf("ownership marker = %#v, want preserved empty marker", client.files[markerPath])
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", client.deleteCalls)
	}
}

func TestFilesystemTextFileDeleteOwnedFile(t *testing.T) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "managed\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, content)
	client.setTestOwnershipMarker(t)
	resource := &filesystemTextFileResource{client: client}

	request := frameworkresource.DeleteRequest{
		State: testFilesystemTextFileState(t, content, true, true),
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(t.Context(), request, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if len(client.files) != 0 {
		t.Fatalf("remaining files = %#v", client.files)
	}
	wantCalls := []string{filePath, filesystemTextFileMarkerPath(filePath)}
	if fmt.Sprint(client.deleteCalls) != fmt.Sprint(wantCalls) {
		t.Fatalf("delete calls = %#v, want %#v", client.deleteCalls, wantCalls)
	}
}

func TestFilesystemTextFileDeleteRefusesContentDrift(t *testing.T) {
	t.Parallel()

	const (
		filePath          = "public_html/site.txt"
		configuredContent = "managed\n"
		driftedContent    = "external\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, driftedContent)
	client.setTestOwnershipMarker(t)
	resource := &filesystemTextFileResource{client: client}

	request := frameworkresource.DeleteRequest{
		State: testFilesystemTextFileState(t, driftedContent, true, false),
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(t.Context(), request, response)
	if !response.Diagnostics.HasError() {
		t.Fatal("Delete() returned no error for drifted content")
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", client.deleteCalls)
	}
	if _, exists := client.files[filePath]; !exists {
		t.Fatal("Delete() removed the drifted target")
	}
}

func TestFilesystemTextFileDeletePreservesUnownedImport(t *testing.T) {
	t.Parallel()

	const (
		filePath = "public_html/site.txt"
		content  = "imported\n"
	)
	client := newFakeFilesystemTextFileClient()
	client.setFile(filePath, content)
	resource := &filesystemTextFileResource{client: client}

	request := frameworkresource.DeleteRequest{
		State: testFilesystemTextFileState(t, content, false, false),
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(t.Context(), request, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
	}
	if len(client.deleteCalls) != 0 {
		t.Fatalf("delete calls = %#v, want none", client.deleteCalls)
	}
	if _, exists := client.files[filePath]; !exists {
		t.Fatal("Delete() removed an unowned import")
	}
}

func TestFilesystemTextFileModelMapping(t *testing.T) {
	t.Parallel()

	textFile := fileman.TextFile{
		Entry: fileman.Entry{
			Path:         "public_html/site.txt",
			AbsolutePath: "/home/example/public_html/site.txt",
			Type:         fileman.EntryTypeFile,
			Permissions:  "0644",
			SizeBytes:    int64(len([]byte("caf\u00e9\n"))),
		},
		Content: "caf\u00e9\n",
	}

	resourceModel := FilesystemTextFileResourceModel{}
	applyFilesystemTextFileToResourceModel(
		&resourceModel,
		textFile,
		true,
		true,
	)
	if resourceModel.Path.ValueString() != textFile.Entry.Path ||
		resourceModel.Content.ValueString() != textFile.Content ||
		resourceModel.AbsolutePath.ValueString() != textFile.Entry.AbsolutePath ||
		resourceModel.Permissions.ValueString() != textFile.Entry.Permissions ||
		resourceModel.SizeBytes.ValueInt64() != textFile.Entry.SizeBytes ||
		resourceModel.ContentSHA256.ValueString() !=
			filesystemTextFileContentSHA256(textFile.Content) ||
		!resourceModel.Owned.ValueBool() ||
		!resourceModel.ContentMatchesOwnershipMarker.ValueBool() {
		t.Fatalf("resource model = %#v", resourceModel)
	}

	dataSourceModel := filesystemTextFileToDataSourceModel(textFile)
	if dataSourceModel.Path.ValueString() != textFile.Entry.Path ||
		dataSourceModel.Content.ValueString() != textFile.Content ||
		dataSourceModel.AbsolutePath.ValueString() != textFile.Entry.AbsolutePath ||
		dataSourceModel.Permissions.ValueString() != textFile.Entry.Permissions ||
		dataSourceModel.SizeBytes.ValueInt64() != textFile.Entry.SizeBytes ||
		dataSourceModel.ContentSHA256.ValueString() !=
			filesystemTextFileContentSHA256(textFile.Content) {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}

func testFilesystemTextFileState(
	t *testing.T,
	content string,
	owned bool,
	contentMatchesOwnershipMarker bool,
) tfsdk.State {
	t.Helper()

	const filePath = "public_html/site.txt"
	response := &frameworkresource.SchemaResponse{}
	NewFilesystemTextFileResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	state := tfsdk.State{Schema: response.Schema}
	diagnostics := state.Set(t.Context(), &FilesystemTextFileResourceModel{
		Path:         types.StringValue(filePath),
		Content:      types.StringValue(content),
		AbsolutePath: types.StringValue(path.Join("/home/example", filePath)),
		Permissions:  types.StringValue("0644"),
		SizeBytes:    types.Int64Value(int64(len([]byte(content)))),
		ContentSHA256: types.StringValue(
			filesystemTextFileContentSHA256(content),
		),
		Owned: types.BoolValue(owned),
		ContentMatchesOwnershipMarker: types.BoolValue(
			contentMatchesOwnershipMarker,
		),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	return state
}

type fakeFilesystemTextFileClient struct {
	files            map[string]fileman.TextFile
	sizeOverrides    map[string]int64
	createErrors     map[string]error
	deleteErrors     map[string]error
	deleteCalls      []string
	getTextFileCalls int
}

func newFakeFilesystemTextFileClient() *fakeFilesystemTextFileClient {
	return &fakeFilesystemTextFileClient{
		files:         make(map[string]fileman.TextFile),
		sizeOverrides: make(map[string]int64),
		createErrors:  make(map[string]error),
		deleteErrors:  make(map[string]error),
	}
}

func (c *fakeFilesystemTextFileClient) setFile(
	filePath string,
	content string,
) {
	c.files[filePath] = fileman.TextFile{
		Entry: fileman.Entry{
			Path:         filePath,
			AbsolutePath: path.Join("/home/example", filePath),
			Type:         fileman.EntryTypeFile,
			Permissions:  "0644",
			SizeBytes:    int64(len([]byte(content))),
		},
		Content: content,
	}
}

func (c *fakeFilesystemTextFileClient) setTestOwnershipMarker(t *testing.T) {
	t.Helper()

	const (
		filePath = "public_html/site.txt"
		content  = "managed\n"
	)
	marker, err := newFilesystemTextFileMarker(
		filePath,
		testFilesystemTextFileOwnershipToken,
		content,
	)
	if err != nil {
		t.Fatalf("newFilesystemTextFileMarker() error: %v", err)
	}
	markerContent, err := filesystemTextFileMarkerContent(marker)
	if err != nil {
		t.Fatalf("filesystemTextFileMarkerContent() error: %v", err)
	}
	c.setFile(filesystemTextFileMarkerPath(filePath), markerContent)
}

func (c *fakeFilesystemTextFileClient) LockMutations() func() {
	return func() {}
}

func (c *fakeFilesystemTextFileClient) GetEntry(
	_ context.Context,
	filePath string,
) (*fileman.Entry, error) {
	textFile, exists := c.files[filePath]
	if !exists {
		return nil, nil
	}
	entry := textFile.Entry
	if size, exists := c.sizeOverrides[filePath]; exists {
		entry.SizeBytes = size
	}

	return &entry, nil
}

func (c *fakeFilesystemTextFileClient) GetTextFile(
	_ context.Context,
	filePath string,
) (*fileman.TextFile, error) {
	c.getTextFileCalls++
	textFile, exists := c.files[filePath]
	if !exists {
		return nil, nil
	}
	textFileCopy := textFile
	if size, exists := c.sizeOverrides[filePath]; exists {
		textFileCopy.Entry.SizeBytes = size
	}

	return &textFileCopy, nil
}

func (c *fakeFilesystemTextFileClient) CreateEmptyTextFile(
	_ context.Context,
	filePath string,
) (*fileman.TextFile, error) {
	if _, exists := c.files[filePath]; exists {
		return nil, fmt.Errorf("path %q already exists", filePath)
	}
	c.setFile(filePath, "")
	if err := c.createErrors[filePath]; err != nil {
		return nil, err
	}

	return c.GetTextFile(context.Background(), filePath)
}

func (c *fakeFilesystemTextFileClient) SaveTextFile(
	_ context.Context,
	filePath string,
	content string,
) (*fileman.TextFile, error) {
	if _, exists := c.files[filePath]; !exists {
		return nil, fmt.Errorf("path %q does not exist", filePath)
	}
	c.setFile(filePath, content)

	return c.GetTextFile(context.Background(), filePath)
}

func (c *fakeFilesystemTextFileClient) DeletePath(
	_ context.Context,
	filePath string,
) error {
	c.deleteCalls = append(c.deleteCalls, filePath)
	if err := c.deleteErrors[filePath]; err != nil {
		return err
	}
	delete(c.files, filePath)

	return nil
}

type fakeFilesystemTextFilePrivateState struct {
	values map[string][]byte
}

func newFakeFilesystemTextFilePrivateState() *fakeFilesystemTextFilePrivateState {
	return &fakeFilesystemTextFilePrivateState{
		values: make(map[string][]byte),
	}
}

func (s *fakeFilesystemTextFilePrivateState) GetKey(
	_ context.Context,
	key string,
) ([]byte, diag.Diagnostics) {
	return append([]byte(nil), s.values[key]...), nil
}

func (s *fakeFilesystemTextFilePrivateState) SetKey(
	_ context.Context,
	key string,
	value []byte,
) diag.Diagnostics {
	s.values[key] = append([]byte(nil), value...)

	return nil
}
