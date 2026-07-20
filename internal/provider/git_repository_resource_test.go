package provider

import (
	"context"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	testresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelversioncontrol "terraform-provider-cpanel/internal/cpanel/versioncontrol"
)

func TestGitRepositoryResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewGitRepositoryResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	for _, attributeName := range []string{"name", "repository_root"} {
		attribute, ok := response.Schema.Attributes[attributeName].(resourceschema.StringAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required string", attributeName)
		}
	}

	sourceURL, ok := response.Schema.Attributes["source_repository_url"].(resourceschema.StringAttribute)
	if !ok || !sourceURL.Optional || !sourceURL.Computed || !sourceURL.Sensitive {
		t.Fatal("source_repository_url must be optional, computed, and sensitive")
	}

	deleteContents, ok := response.Schema.Attributes["delete_contents_on_destroy"].(resourceschema.BoolAttribute)
	if !ok || !deleteContents.Optional || !deleteContents.Computed {
		t.Fatal("delete_contents_on_destroy must be optional and computed")
	}
}

func TestGitSourceRepositoryURLChanged(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		state types.String
		plan  types.String
		want  bool
	}{
		"same URL": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
		},
		"different URL": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringValue("https://github.com/octocat/Spoon-Knife.git"),
			want:  true,
		},
		"unknown state": {
			state: types.StringUnknown(),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
		},
		"unknown plan": {
			state: types.StringValue("https://github.com/octocat/Hello-World.git"),
			plan:  types.StringUnknown(),
		},
		"null to URL": {
			state: types.StringNull(),
			plan:  types.StringValue("https://github.com/octocat/Hello-World.git"),
			want:  true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := gitSourceRepositoryURLChanged(
				GitRepositoryResourceModel{
					SourceRepositoryURL: test.state,
				},
				GitRepositoryResourceModel{
					SourceRepositoryURL: test.plan,
				},
			)
			if got != test.want {
				t.Fatalf(
					"gitSourceRepositoryURLChanged() = %t, want %t",
					got,
					test.want,
				)
			}
		})
	}
}

func TestGitSourceReplacementRequiresDeletion(t *testing.T) {
	t.Parallel()

	sourceURL := types.StringValue(
		"https://github.com/octocat/Hello-World.git",
	)
	replacementURL := types.StringValue(
		"https://github.com/octocat/Spoon-Knife.git",
	)

	tests := map[string]struct {
		stateRoot types.String
		planRoot  types.String
		stateURL  types.String
		planURL   types.String
		want      bool
	}{
		"same root and changed source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/website"),
			stateURL:  sourceURL,
			planURL:   replacementURL,
			want:      true,
		},
		"changed root and changed source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/replacement"),
			stateURL:  sourceURL,
			planURL:   replacementURL,
		},
		"same root and same source": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringValue("repositories/website"),
			stateURL:  sourceURL,
			planURL:   sourceURL,
		},
		"unknown planned root remains conservative": {
			stateRoot: types.StringValue("repositories/website"),
			planRoot:  types.StringUnknown(),
			stateURL:  sourceURL,
			planURL:   replacementURL,
			want:      true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := gitSourceReplacementRequiresDeletion(
				GitRepositoryResourceModel{
					RepositoryRoot:      test.stateRoot,
					SourceRepositoryURL: test.stateURL,
				},
				GitRepositoryResourceModel{
					RepositoryRoot:      test.planRoot,
					SourceRepositoryURL: test.planURL,
				},
			)
			if got != test.want {
				t.Fatalf(
					"gitSourceReplacementRequiresDeletion() = %t, want %t",
					got,
					test.want,
				)
			}
		})
	}
}

func TestGitRepositoryCreationErrorRedactsSensitiveSourceURL(t *testing.T) {
	t.Parallel()

	const secret = "private-repository-name"

	remoteErr := errors.New("clone failed for " + secret)
	got := gitRepositoryCreationError(
		remoteErr,
		"https://example.test/"+secret+".git",
	)
	if got == nil {
		t.Fatal("gitRepositoryCreationError() = nil, want error")
	}
	if strings.Contains(got.Error(), secret) {
		t.Fatalf("gitRepositoryCreationError() leaked %q", secret)
	}

	if got = gitRepositoryCreationError(remoteErr, ""); got != remoteErr {
		t.Fatalf(
			"gitRepositoryCreationError() without a source URL = %v, want original error",
			got,
		)
	}
}

func TestGitRepositoryReadErrorRedactsRemoteDetails(t *testing.T) {
	t.Parallel()

	const secret = "private-source-url-that-must-not-leak"
	err := &cpanel.APIError{
		API:      "UAPI",
		Module:   "VersionControl",
		Function: "retrieve",
		Messages: []string{"cannot inspect https://example.test/" + secret},
	}

	got := gitRepositoryReadError(err)
	if got == nil {
		t.Fatal("gitRepositoryReadError() = nil, want error")
	}
	if strings.Contains(got.Error(), secret) {
		t.Fatalf("gitRepositoryReadError() leaked %q", secret)
	}
}

func TestGitRepositoryDestructiveDeletionRequiresCompleteIdentity(
	t *testing.T,
) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	tests := map[string]func(*cpanelversioncontrol.Repository){
		"name": func(repository *cpanelversioncontrol.Repository) {
			repository.Name = "replacement"
		},
		"repository root": func(repository *cpanelversioncontrol.Repository) {
			repository.RepositoryRoot = "repositories/replacement"
		},
		"absolute root": func(repository *cpanelversioncontrol.Repository) {
			repository.AbsoluteRoot = "/home/test/repositories/replacement"
		},
		"type": func(repository *cpanelversioncontrol.Repository) {
			repository.Type = "other"
		},
		"source repository name": func(repository *cpanelversioncontrol.Repository) {
			repository.SourceRepositoryName = "upstream"
		},
		"source repository URL": func(repository *cpanelversioncontrol.Repository) {
			repository.SourceRepositoryURL = "https://example.test/replacement.git"
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			current := expected
			mutate(&current)
			client := &fakeGitRepositoryClient{
				repository: &current,
			}
			resource := &gitRepositoryResource{client: client}

			err := resource.deleteRepositoryAndContents(
				t.Context(),
				expected,
			)
			if err == nil {
				t.Fatal("deleteRepositoryAndContents() error = nil")
			}
			if client.deleteCalls != 0 {
				t.Fatalf(
					"Delete() calls = %d, want 0",
					client.deleteCalls,
				)
			}
			if client.deleteDirectoryCalls != 0 {
				t.Fatalf(
					"DeleteDirectory() calls = %d, want 0",
					client.deleteDirectoryCalls,
				)
			}
		})
	}
}

func TestGitRepositoryDestructiveDeletionPreservesUnattributedDirectory(
	t *testing.T,
) {
	t.Parallel()

	client := &fakeGitRepositoryClient{}
	resource := &gitRepositoryResource{client: client}

	if err := resource.deleteRepositoryAndContents(
		t.Context(),
		testGitRepositoryIdentity(),
	); err == nil {
		t.Fatal("deleteRepositoryAndContents() error = nil")
	}
	if client.deleteCalls != 0 || client.deleteDirectoryCalls != 0 {
		t.Fatalf(
			"destructive calls = unregister:%d directory:%d, want 0 and 0",
			client.deleteCalls,
			client.deleteDirectoryCalls,
		)
	}
}

func TestGitRepositoryDestructiveDeletionReconcilesDeleteResult(
	t *testing.T,
) {
	t.Parallel()

	deleteFailure := errors.New("delete response lost")
	readFailure := errors.New("inventory unavailable")
	expected := testGitRepositoryIdentity()
	replacement := expected
	replacement.Name = "concurrent replacement"

	tests := map[string]struct {
		deleteErr               error
		repositoryAfterDelete   *cpanelversioncontrol.Repository
		secondGetErr            error
		wantErr                 bool
		wantDeleteDirectoryCall bool
	}{
		"success confirmed absent": {
			wantDeleteDirectoryCall: true,
		},
		"delete error but absence confirmed": {
			deleteErr:               deleteFailure,
			wantDeleteDirectoryCall: true,
		},
		"false success leaves expected repository": {
			repositoryAfterDelete: &expected,
			wantErr:               true,
		},
		"delete error leaves expected repository": {
			deleteErr:             deleteFailure,
			repositoryAfterDelete: &expected,
			wantErr:               true,
		},
		"repository replaced during delete": {
			repositoryAfterDelete: &replacement,
			wantErr:               true,
		},
		"post-delete read fails": {
			secondGetErr: readFailure,
			wantErr:      true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			current := expected
			client := &fakeGitRepositoryClient{
				repository:            &current,
				deleteErr:             test.deleteErr,
				deleteSetsRepository:  true,
				repositoryAfterDelete: test.repositoryAfterDelete,
				getErrors: map[int]error{
					2: test.secondGetErr,
				},
			}
			resource := &gitRepositoryResource{client: client}

			err := resource.deleteRepositoryAndContents(
				t.Context(),
				expected,
			)
			if (err != nil) != test.wantErr {
				t.Fatalf(
					"deleteRepositoryAndContents() error = %v, wantError %t",
					err,
					test.wantErr,
				)
			}
			if client.getCalls != 2 {
				t.Fatalf(
					"Get() calls = %d, want mandatory pre/post reads",
					client.getCalls,
				)
			}
			if client.deleteCalls != 1 {
				t.Fatalf(
					"Delete() calls = %d, want 1",
					client.deleteCalls,
				)
			}
			wantDirectoryCalls := 0
			if test.wantDeleteDirectoryCall {
				wantDirectoryCalls = 1
			}
			if client.deleteDirectoryCalls != wantDirectoryCalls {
				t.Fatalf(
					"DeleteDirectory() calls = %d, want %d",
					client.deleteDirectoryCalls,
					wantDirectoryCalls,
				)
			}
		})
	}
}

func TestGitRepositoryDestructiveDeletionRetriesPendingTasks(t *testing.T) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	current := expected
	client := &fakeGitRepositoryClient{
		repository:           &current,
		deleteSetsRepository: true,
		deleteSucceedsOnCall: 2,
		deleteErrors: map[int]error{
			1: &cpanel.APIError{
				API:      "UAPI",
				Module:   "VersionControl",
				Function: "delete",
				Messages: []string{
					expected.AbsoluteRoot +
						" can not be deleted because there are tasks pending.",
				},
			},
		},
	}
	resource := &gitRepositoryResource{client: client}

	if err := resource.deleteRepositoryAndContents(
		t.Context(),
		expected,
	); err != nil {
		t.Fatalf("deleteRepositoryAndContents() error = %v", err)
	}
	if client.getCalls != 3 ||
		client.deleteCalls != 2 ||
		client.deleteDirectoryCalls != 1 {
		t.Fatalf(
			"calls = get:%d delete:%d directory:%d, want 3, 2, 1",
			client.getCalls,
			client.deleteCalls,
			client.deleteDirectoryCalls,
		)
	}
}

func TestGitRepositoryCreateRollbackUsesCreatedRepositoryIdentity(
	t *testing.T,
) {
	t.Parallel()

	created := testGitRepositoryIdentity()

	t.Run("matching created repository is removed", func(t *testing.T) {
		t.Parallel()

		current := created
		client := &fakeGitRepositoryClient{
			repository:           &current,
			deleteSetsRepository: true,
		}
		resource := &gitRepositoryResource{client: client}

		if err := resource.deleteRepositoryAndContents(
			t.Context(),
			created,
		); err != nil {
			t.Fatalf("deleteRepositoryAndContents() error = %v", err)
		}
		if client.deleteCalls != 1 || client.deleteDirectoryCalls != 1 {
			t.Fatalf(
				"destructive calls = unregister:%d directory:%d, want 1 and 1",
				client.deleteCalls,
				client.deleteDirectoryCalls,
			)
		}
	})

	t.Run("replacement at same root is preserved", func(t *testing.T) {
		t.Parallel()

		replacement := created
		replacement.SourceRepositoryURL = "https://example.test/replacement.git"
		client := &fakeGitRepositoryClient{
			repository: &replacement,
		}
		resource := &gitRepositoryResource{client: client}

		if err := resource.deleteRepositoryAndContents(
			t.Context(),
			created,
		); err == nil {
			t.Fatal("deleteRepositoryAndContents() error = nil")
		}
		if client.deleteCalls != 0 || client.deleteDirectoryCalls != 0 {
			t.Fatalf(
				"destructive calls = unregister:%d directory:%d, want 0 and 0",
				client.deleteCalls,
				client.deleteDirectoryCalls,
			)
		}
	})
}

func TestGitRepositoryCreateReconcilesAmbiguousAppliedResponse(t *testing.T) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	client := &fakeGitRepositoryClient{
		createErr:        errors.New("create response lost"),
		createRepository: &expected,
	}
	subject := &gitRepositoryResource{client: client}
	schema := gitRepositoryResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schema}
	model := gitRepositoryCreateTestModel(expected)
	if diagnostics := plan.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schema},
	}

	subject.Create(
		t.Context(),
		frameworkresource.CreateRequest{Plan: plan},
		response,
	)

	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics: %v", response.Diagnostics)
	}
	if client.createCalls != 1 || client.deleteCalls != 0 ||
		client.deleteDirectoryCalls != 0 {
		t.Fatalf(
			"calls = create:%d delete:%d directory:%d",
			client.createCalls,
			client.deleteCalls,
			client.deleteDirectoryCalls,
		)
	}
	var state GitRepositoryResourceModel
	response.Diagnostics.Append(response.State.Get(t.Context(), &state)...)
	if response.Diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", response.Diagnostics)
	}
	if state.RepositoryRoot.ValueString() != expected.RepositoryRoot ||
		state.AbsoluteRepositoryRoot.ValueString() != expected.AbsoluteRoot ||
		state.SourceRepositoryURL.ValueString() !=
			expected.SourceRepositoryURL {
		t.Fatalf("created state = %#v", state)
	}
}

func TestGitRepositorySourceCloneWaitsForStableInventory(t *testing.T) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	temporarilyVisible := expected
	temporarilyVisible.Branch = ""
	temporarilyVisible.AvailableBranches = nil
	ready := expected
	ready.Branch = "main"
	ready.AvailableBranches = []string{"main"}
	client := &fakeGitRepositoryClient{
		repositoriesByGetCall: map[int]*cpanelversioncontrol.Repository{
			1: &temporarilyVisible,
			2: nil,
			3: &ready,
			4: &ready,
			5: &ready,
			6: &ready,
			7: &ready,
			8: &ready,
		},
	}
	waitCalls := 0
	subject := &gitRepositoryResource{
		client: client,
		waitForCloneRetry: func(context.Context) error {
			waitCalls++

			return nil
		},
	}

	repository, err := subject.verifyCreatedRepository(
		t.Context(),
		cpanelversioncontrol.Definition{
			Name:                expected.Name,
			RepositoryRoot:      expected.RepositoryRoot,
			SourceRepositoryURL: expected.SourceRepositoryURL,
		},
	)
	if err != nil {
		t.Fatalf("verifyCreatedRepository() error = %v", err)
	}
	if repository == nil || repository.Branch != "main" {
		t.Fatalf("verifyCreatedRepository() = %#v", repository)
	}
	if client.getCalls != 8 || waitCalls != 7 {
		t.Fatalf(
			"calls = get:%d wait:%d, want 8 and 7",
			client.getCalls,
			waitCalls,
		)
	}
}

func TestGitRepositorySourceCloneRejectsUnexpectedIdentity(t *testing.T) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	replacement := expected
	replacement.Name = "Concurrent repository"
	client := &fakeGitRepositoryClient{
		repository: &replacement,
	}
	subject := &gitRepositoryResource{client: client}

	_, err := subject.verifyCreatedRepository(
		t.Context(),
		cpanelversioncontrol.Definition{
			Name:                expected.Name,
			RepositoryRoot:      expected.RepositoryRoot,
			SourceRepositoryURL: expected.SourceRepositoryURL,
		},
	)
	if err == nil {
		t.Fatal("verifyCreatedRepository() error = nil")
	}
	if client.getCalls != 1 {
		t.Fatalf("Get() calls = %d, want 1", client.getCalls)
	}
}

func TestGitRepositoryCreateDoesNotReconcileDeterministicError(t *testing.T) {
	t.Parallel()

	expected := testGitRepositoryIdentity()
	client := &fakeGitRepositoryClient{
		createErr: &cpanel.APIError{
			API:      "UAPI",
			Module:   "VersionControl",
			Function: "create",
			Messages: []string{"rejected"},
		},
		createRepository: &expected,
	}
	subject := &gitRepositoryResource{client: client}
	schema := gitRepositoryResourceTestSchema(t)
	plan := tfsdk.Plan{Schema: schema}
	model := gitRepositoryCreateTestModel(expected)
	if diagnostics := plan.Set(t.Context(), &model); diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schema},
	}

	subject.Create(
		t.Context(),
		frameworkresource.CreateRequest{Plan: plan},
		response,
	)

	if !response.Diagnostics.HasError() {
		t.Fatal("Create() diagnostics contain no error")
	}
	if client.getCalls != 1 {
		t.Fatalf("Get() calls = %d, want only the preflight read", client.getCalls)
	}
	if client.deleteCalls != 0 || client.deleteDirectoryCalls != 0 {
		t.Fatalf(
			"destructive calls = unregister:%d directory:%d, want 0 and 0",
			client.deleteCalls,
			client.deleteDirectoryCalls,
		)
	}
}

func TestGitRepositoryNameRollbackRequiresAttemptedIdentity(t *testing.T) {
	t.Parallel()

	previous := testGitRepositoryIdentity()
	attempted := previous
	attempted.Name = "Terraform repository updated"

	t.Run("already restored", func(t *testing.T) {
		t.Parallel()

		current := previous
		client := &fakeGitRepositoryClient{repository: &current}
		resource := &gitRepositoryResource{client: client}

		if err := resource.restoreGitRepositoryName(
			t.Context(),
			previous,
			attempted,
		); err != nil {
			t.Fatalf("restoreGitRepositoryName() error: %v", err)
		}
		if len(client.updateNames) != 0 {
			t.Fatalf("update names = %#v, want none", client.updateNames)
		}
	})

	t.Run("attempted identity is restored", func(t *testing.T) {
		t.Parallel()

		current := attempted
		client := &fakeGitRepositoryClient{
			repository:     &current,
			updateSetsName: true,
		}
		resource := &gitRepositoryResource{client: client}

		if err := resource.restoreGitRepositoryName(
			t.Context(),
			previous,
			attempted,
		); err != nil {
			t.Fatalf("restoreGitRepositoryName() error: %v", err)
		}
		if len(client.updateNames) != 1 ||
			client.updateNames[0] != previous.Name {
			t.Fatalf(
				"update names = %#v, want [%q]",
				client.updateNames,
				previous.Name,
			)
		}
		if client.repository == nil ||
			!gitRepositoryIdentitiesEqual(*client.repository, previous) {
			t.Fatalf("restored repository = %#v, want %#v", client.repository, previous)
		}
	})

	t.Run("concurrent identity is preserved", func(t *testing.T) {
		t.Parallel()

		concurrent := attempted
		concurrent.Name = "Concurrent repository name"
		client := &fakeGitRepositoryClient{repository: &concurrent}
		resource := &gitRepositoryResource{client: client}

		if err := resource.restoreGitRepositoryName(
			t.Context(),
			previous,
			attempted,
		); err == nil {
			t.Fatal("restoreGitRepositoryName() returned no error")
		}
		if len(client.updateNames) != 0 {
			t.Fatalf("update names = %#v, want none", client.updateNames)
		}
		if client.repository == nil ||
			!gitRepositoryIdentitiesEqual(*client.repository, concurrent) {
			t.Fatalf(
				"concurrent repository = %#v, want %#v",
				client.repository,
				concurrent,
			)
		}
	})
}

func TestGitRepositoryIdentityFromResourceModelRequiresRefreshableState(
	t *testing.T,
) {
	t.Parallel()

	validModel := GitRepositoryResourceModel{
		Name:                   types.StringValue("Terraform repository"),
		RepositoryRoot:         types.StringValue("repositories/example"),
		AbsoluteRepositoryRoot: types.StringValue("/home/test/repositories/example"),
		Type:                   types.StringValue("git"),
		SourceRepositoryURL:    types.StringNull(),
		SourceRepositoryName:   types.StringNull(),
	}

	identity, err := gitRepositoryIdentityFromResourceModel(validModel)
	if err != nil {
		t.Fatalf("gitRepositoryIdentityFromResourceModel() error = %v", err)
	}
	if identity.Name != validModel.Name.ValueString() ||
		identity.RepositoryRoot != validModel.RepositoryRoot.ValueString() ||
		identity.AbsoluteRoot != validModel.AbsoluteRepositoryRoot.ValueString() ||
		identity.Type != validModel.Type.ValueString() ||
		identity.SourceRepositoryName != "" ||
		identity.SourceRepositoryURL != "" {
		t.Fatalf(
			"gitRepositoryIdentityFromResourceModel() = %#v",
			identity,
		)
	}

	invalidModel := validModel
	invalidModel.AbsoluteRepositoryRoot = types.StringUnknown()
	if _, err := gitRepositoryIdentityFromResourceModel(invalidModel); err == nil {
		t.Fatal("gitRepositoryIdentityFromResourceModel() error = nil")
	}
}

type fakeGitRepositoryClient struct {
	repository            *cpanelversioncontrol.Repository
	createRepository      *cpanelversioncontrol.Repository
	repositoryAfterDelete *cpanelversioncontrol.Repository
	repositoriesByGetCall map[int]*cpanelversioncontrol.Repository
	getErrors             map[int]error
	createErr             error
	deleteErr             error
	deleteErrors          map[int]error
	deleteDirectoryErr    error
	updateErr             error
	deleteSetsRepository  bool
	deleteSucceedsOnCall  int
	updateSetsName        bool
	getCalls              int
	createCalls           int
	deleteCalls           int
	deleteDirectoryCalls  int
	updateNames           []string
}

func (c *fakeGitRepositoryClient) Get(
	_ context.Context,
	_ string,
) (*cpanelversioncontrol.Repository, error) {
	c.getCalls++
	if err := c.getErrors[c.getCalls]; err != nil {
		return nil, err
	}
	if repository, ok := c.repositoriesByGetCall[c.getCalls]; ok {
		if repository == nil {
			return nil, nil
		}

		repositoryCopy := *repository

		return &repositoryCopy, nil
	}
	if c.repository == nil {
		return nil, nil
	}

	repository := *c.repository

	return &repository, nil
}

func (c *fakeGitRepositoryClient) RootExists(
	context.Context,
	string,
) (bool, error) {
	return c.repository != nil, nil
}

func (c *fakeGitRepositoryClient) Create(
	context.Context,
	cpanelversioncontrol.Definition,
) (*cpanelversioncontrol.Repository, error) {
	c.createCalls++
	if c.createRepository != nil {
		repository := *c.createRepository
		c.repository = &repository
	}
	if c.repository == nil {
		return nil, c.createErr
	}

	repository := *c.repository

	return &repository, c.createErr
}

func (c *fakeGitRepositoryClient) Update(
	_ context.Context,
	_ string,
	name string,
) (*cpanelversioncontrol.Repository, error) {
	c.updateNames = append(c.updateNames, name)
	if c.updateSetsName && c.repository != nil {
		c.repository.Name = name
	}
	if c.repository == nil {
		return nil, c.updateErr
	}

	repository := *c.repository

	return &repository, c.updateErr
}

func (c *fakeGitRepositoryClient) Delete(
	context.Context,
	string,
) error {
	c.deleteCalls++
	if c.deleteSetsRepository &&
		(c.deleteSucceedsOnCall == 0 ||
			c.deleteCalls >= c.deleteSucceedsOnCall) {
		if c.repositoryAfterDelete == nil {
			c.repository = nil
		} else {
			repository := *c.repositoryAfterDelete
			c.repository = &repository
		}
	}
	if err := c.deleteErrors[c.deleteCalls]; err != nil {
		return err
	}

	return c.deleteErr
}

func (c *fakeGitRepositoryClient) DeleteDirectory(
	context.Context,
	string,
) error {
	c.deleteDirectoryCalls++

	return c.deleteDirectoryErr
}

func testGitRepositoryIdentity() cpanelversioncontrol.Repository {
	return cpanelversioncontrol.Repository{
		Name:                 "Terraform repository",
		RepositoryRoot:       "repositories/example",
		AbsoluteRoot:         "/home/test/repositories/example",
		Type:                 "git",
		SourceRepositoryName: "origin",
		SourceRepositoryURL:  "https://example.test/source.git",
	}
}

func gitRepositoryResourceTestSchema(t *testing.T) resourceschema.Schema {
	t.Helper()

	response := &frameworkresource.SchemaResponse{}
	NewGitRepositoryResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	return response.Schema
}

func gitRepositoryCreateTestModel(
	repository cpanelversioncontrol.Repository,
) GitRepositoryResourceModel {
	return GitRepositoryResourceModel{
		Name:                    types.StringValue(repository.Name),
		RepositoryRoot:          types.StringValue(repository.RepositoryRoot),
		SourceRepositoryURL:     nullableString(repository.SourceRepositoryURL),
		DeleteContentsOnDestroy: types.BoolValue(false),
		AbsoluteRepositoryRoot:  types.StringUnknown(),
		Type:                    types.StringUnknown(),
		Branch:                  types.StringUnknown(),
		AvailableBranches:       types.SetUnknown(types.StringType),
		ReadOnlyCloneURLs:       types.SetUnknown(types.StringType),
		ReadWriteCloneURLs:      types.SetUnknown(types.StringType),
		SourceRepositoryName:    types.StringUnknown(),
		Deployable:              types.BoolUnknown(),
	}
}

func TestAccGitRepositoryResource(t *testing.T) {
	const resourceName = "cpanel_git_repository.test"

	initialRoot := testAccGitRepositoryRoot("resource")
	replacementRoot := testAccGitRepositoryRoot("replacement")
	initialName := "Terraform Git initial"
	updatedName := "Terraform Git updated"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			initialRoot,
			replacementRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					initialName,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"name",
						initialName,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"repository_root",
						initialRoot,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"type",
						"git",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"deployable",
						"false",
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"delete_contents_on_destroy",
						"true",
					),
					testresource.TestCheckResourceAttrSet(
						resourceName,
						"absolute_repository_root",
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:           initialName,
							RepositoryRoot: initialRoot,
						},
					),
				),
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        initialRoot,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "repository_root",
				ImportStateVerifyIgnore: []string{
					"delete_contents_on_destroy",
				},
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				PreConfig: func() {
					testAccSetGitRepositoryName(
						t,
						initialRoot,
						"Terraform Git external",
					)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					initialRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: initialRoot,
					},
				),
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					replacementRoot,
					updatedName,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testAccCheckGitRepositoryMissing(initialRoot),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:           updatedName,
							RepositoryRoot: replacementRoot,
						},
					),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, replacementRoot)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccGitRepositoryResourceConfig(
					replacementRoot,
					updatedName,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           updatedName,
						RepositoryRoot: replacementRoot,
					},
				),
			},
		},
	})
}

func TestAccGitRepositoryRetainsContentsByDefault(t *testing.T) {
	repositoryRoot := testAccGitRepositoryRoot("retain")
	repositoryName := "Terraform Git retained"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositoryResourceConfig(
					repositoryRoot,
					repositoryName,
					false,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           repositoryName,
						RepositoryRoot: repositoryRoot,
					},
				),
			},
			{
				Config: `provider "cpanel" {}`,
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:           repositoryName,
						RepositoryRoot: repositoryRoot,
					},
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, repositoryRoot)
				},
				Config: `provider "cpanel" {}`,
				Check:  testAccCheckGitRepositoryMissing(repositoryRoot),
			},
		},
	})
}

func TestAccGitRepositorySourceClone(t *testing.T) {
	const (
		resourceName         = "cpanel_git_repository.test"
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
	)

	repositoryRoot := testAccRegisterGitRepositoryRoot(path.Join(
		testAccGitRepositoryRoot("source-parent"),
		testAccGitRepositoryRoot("source"),
	))
	repositoryName := "Terraform Git source clone"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					sourceURL,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_url",
						sourceURL,
					),
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_name",
						"origin",
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:                repositoryName,
							RepositoryRoot:      repositoryRoot,
							SourceRepositoryURL: sourceURL,
						},
					),
					testAccCheckDirectoryEntryExists(
						repositoryRoot,
						"README",
					),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        repositoryRoot,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "repository_root",
				ImportStateVerifyIgnore: []string{
					"branch",
					"delete_contents_on_destroy",
				},
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					sourceURL,
					true,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:                repositoryName,
						RepositoryRoot:      repositoryRoot,
						SourceRepositoryURL: sourceURL,
					},
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					replacementSourceURL,
					true,
				),
				Check: testresource.ComposeAggregateTestCheckFunc(
					testresource.TestCheckResourceAttr(
						resourceName,
						"source_repository_url",
						replacementSourceURL,
					),
					testAccCheckGitRepositoryExists(
						cpanelversioncontrol.Definition{
							Name:                repositoryName,
							RepositoryRoot:      repositoryRoot,
							SourceRepositoryURL: replacementSourceURL,
						},
					),
				),
			},
		},
	})
}

func TestAccGitRepositorySourceReplacementRequiresDeletion(t *testing.T) {
	const (
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
	)

	repositoryRoot := testAccRegisterGitRepositoryRoot(path.Join(
		testAccGitRepositoryRoot("source-guard-parent"),
		testAccGitRepositoryRoot("source-guard"),
	))
	repositoryName := "Terraform Git source guard"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			repositoryRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					sourceURL,
					false,
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					repositoryRoot,
					repositoryName,
					replacementSourceURL,
					false,
				),
				ExpectError: regexp.MustCompile(
					"Git source replacement requires prior destructive deletion",
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, repositoryRoot)
				},
				Config: `provider "cpanel" {}`,
			},
		},
	})
}

func TestAccGitRepositorySourceAndRootReplacement(t *testing.T) {
	const (
		sourceURL            = "https://github.com/octocat/Hello-World.git"
		replacementSourceURL = "https://github.com/octocat/Spoon-Knife.git"
	)

	initialRoot := testAccGitRepositoryRoot("source-root-initial")
	replacementRoot := testAccGitRepositoryRoot("source-root-replacement")
	repositoryName := "Terraform Git source and root replacement"

	testresource.Test(t, testresource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckGitRepositoriesDestroyed(
			initialRoot,
			replacementRoot,
		),
		Steps: []testresource.TestStep{
			{
				Config: testAccGitRepositorySourceConfig(
					initialRoot,
					repositoryName,
					sourceURL,
					false,
				),
			},
			{
				Config: testAccGitRepositorySourceConfig(
					replacementRoot,
					repositoryName,
					replacementSourceURL,
					false,
				),
				Check: testAccCheckGitRepositoryExists(
					cpanelversioncontrol.Definition{
						Name:                repositoryName,
						RepositoryRoot:      replacementRoot,
						SourceRepositoryURL: replacementSourceURL,
					},
				),
			},
			{
				PreConfig: func() {
					testAccDeleteGitRepository(t, initialRoot)
					testAccDeleteGitRepository(t, replacementRoot)
				},
				Config: `provider "cpanel" {}`,
			},
		},
	})
}

func testAccGitRepositoryResourceConfig(
	repositoryRoot string,
	name string,
	deleteContentsOnDestroy bool,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = %q
  repository_root            = %q
  delete_contents_on_destroy = %t
}
`, name, repositoryRoot, deleteContentsOnDestroy)
}

func testAccGitRepositorySourceConfig(
	repositoryRoot string,
	name string,
	sourceURL string,
	deleteContentsOnDestroy bool,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_git_repository" "test" {
  name                       = %q
  repository_root            = %q
  source_repository_url      = %q
  delete_contents_on_destroy = %t
}
`, name, repositoryRoot, sourceURL, deleteContentsOnDestroy)
}
