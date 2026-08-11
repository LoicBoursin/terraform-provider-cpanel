package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailMailingListResourceSchema(t *testing.T) {
	t.Parallel()

	response := &frameworkresource.SchemaResponse{}
	NewEmailMailingListResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", response.Diagnostics)
	}

	address, ok := response.Schema.Attributes["address"].(resourceschema.StringAttribute)
	if !ok || !address.Required || len(address.PlanModifiers) == 0 {
		t.Fatal("address must be a required replacement string")
	}

	password, ok := response.Schema.Attributes["password"].(resourceschema.StringAttribute)
	if !ok ||
		!password.Optional ||
		!password.Sensitive ||
		!password.WriteOnly ||
		password.Required ||
		password.Computed ||
		len(password.PlanModifiers) != 0 {
		t.Fatal("password must be an optional sensitive write-only string")
	}
	passwordVersion, ok := response.Schema.Attributes["password_version"].(resourceschema.Int64Attribute)
	if !ok ||
		!passwordVersion.Optional ||
		passwordVersion.Required ||
		passwordVersion.Computed {
		t.Fatal("password_version must be an optional persistent int64")
	}
	deleteOnDestroy, ok := response.Schema.Attributes["delete_on_destroy"].(resourceschema.BoolAttribute)
	if !ok ||
		!deleteOnDestroy.Optional ||
		!deleteOnDestroy.Computed ||
		deleteOnDestroy.Required {
		t.Fatal("delete_on_destroy must be an optional computed bool")
	}

	for _, name := range []string{"advertised", "archive_private"} {
		attribute, ok := response.Schema.Attributes[name].(resourceschema.BoolAttribute)
		if !ok || !attribute.Required {
			t.Fatalf("%s must be a required bool", name)
		}
	}
	subscribePolicy, ok := response.Schema.Attributes["subscribe_policy"].(resourceschema.Int64Attribute)
	if !ok || !subscribePolicy.Required {
		t.Fatal("subscribe_policy must be a required int64")
	}

	for _, name := range []string{
		"private",
		"list_id",
		"administrators",
		"human_disk_used",
	} {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsComputed() ||
			attribute.IsOptional() ||
			attribute.IsRequired() {
			t.Fatalf("%s must be computed-only", name)
		}
	}
}

func TestWaitForMailmanPasswordAcceptanceRetriesUntilExpected(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := testAccWaitForMailmanPasswordAcceptance(
		t.Context(),
		"list@example.test",
		true,
		time.Second,
		time.Nanosecond,
		func(context.Context) (bool, error) {
			attempts++

			return attempts == 3, nil
		},
	)
	if err != nil {
		t.Fatalf("wait for Mailman password acceptance: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("password checks = %d, want 3", attempts)
	}
}

func TestWaitForMailmanPasswordAcceptanceRejectsPermanentMismatch(t *testing.T) {
	t.Parallel()

	err := testAccWaitForMailmanPasswordAcceptance(
		t.Context(),
		"list@example.test",
		true,
		time.Nanosecond,
		time.Hour,
		func(context.Context) (bool, error) {
			return false, nil
		},
	)
	if err == nil || !strings.Contains(
		err.Error(),
		`password acceptance for "list@example.test" is false; want true`,
	) {
		t.Fatalf("wait error = %v, want permanent mismatch", err)
	}
}

func TestEmailMailingListUpdateChangesPasswordInPlace(t *testing.T) {
	t.Parallel()

	current := testEmailMailingList(testEmailMailingListPrivate())
	client := newFakeEmailMailingListClient(current)
	resource := &emailMailingListResource{client: client}
	state := testEmailMailingListModel(t, current)
	plan := state
	plan.PasswordVersion = types.Int64Value(2)
	config := plan
	config.Password = types.StringValue("new-password")

	response := runEmailMailingListUpdate(t, resource, state, plan, config)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics: %v", response.Diagnostics)
	}
	if len(client.passwordCalls) != 1 ||
		client.passwordCalls[0] != "new-password" {
		t.Fatalf(
			"password calls = %#v, want [new-password]",
			client.passwordCalls,
		)
	}
	if len(client.privacyCalls) != 0 {
		t.Fatalf("privacy calls = %#v, want none", client.privacyCalls)
	}
	assertEmailMailingListNoReplacement(t, client)
}

func TestEmailMailingListUpdateChangesPrivacyInPlace(t *testing.T) {
	t.Parallel()

	current := testEmailMailingList(testEmailMailingListPrivate())
	desired := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	client := newFakeEmailMailingListClient(current)
	resource := &emailMailingListResource{client: client}
	state := testEmailMailingListModel(t, current)
	plan := state
	plan.Advertised = types.BoolValue(desired.Advertised)
	plan.ArchivePrivate = types.BoolValue(desired.ArchivePrivate)
	plan.SubscribePolicy = types.Int64Value(desired.SubscribePolicy)
	config := plan
	config.Password = types.StringValue("password")

	response := runEmailMailingListUpdate(t, resource, state, plan, config)
	if response.Diagnostics.HasError() {
		t.Fatalf("Update() diagnostics: %v", response.Diagnostics)
	}
	if len(client.privacyCalls) != 1 ||
		client.privacyCalls[0] != desired {
		t.Fatalf(
			"privacy calls = %#v, want [%#v]",
			client.privacyCalls,
			desired,
		)
	}
	if len(client.passwordCalls) != 0 {
		t.Fatalf("password calls = %#v, want none", client.passwordCalls)
	}
	if actual := mailingListPrivacy(*client.current); actual != desired {
		t.Fatalf("remote privacy = %#v, want %#v", actual, desired)
	}
	assertEmailMailingListNoReplacement(t, client)

	var updated EmailMailingListResourceModel
	diagnostics := response.State.Get(t.Context(), &updated)
	if diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", diagnostics)
	}
	if updated.Private.ValueBool() {
		t.Fatal("computed private is true for mixed public settings")
	}
	if updated.ListID.ValueString() != current.ID {
		t.Fatalf(
			"updated list_id = %q, want %q",
			updated.ListID.ValueString(),
			current.ID,
		)
	}
}

func TestEmailMailingListUpdateValidatesPasswordBeforePrivacyMutation(
	t *testing.T,
) {
	t.Parallel()

	tests := map[string]struct {
		configure func(
			*EmailMailingListResourceModel,
			*EmailMailingListResourceModel,
		)
		wantDiagnostic string
	}{
		"unknown version": {
			configure: func(
				plan *EmailMailingListResourceModel,
				config *EmailMailingListResourceModel,
			) {
				plan.PasswordVersion = types.Int64Unknown()
				config.PasswordVersion = types.Int64Unknown()
				config.Password = types.StringValue("new-password")
			},
			wantDiagnostic: "Unknown mailing list password version",
		},
		"missing password": {
			configure: func(
				plan *EmailMailingListResourceModel,
				config *EmailMailingListResourceModel,
			) {
				plan.PasswordVersion = types.Int64Value(2)
				config.PasswordVersion = types.Int64Value(2)
				config.Password = types.StringNull()
			},
			wantDiagnostic: "Missing mailing list password",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			previous := testEmailMailingListPrivate()
			current := testEmailMailingList(previous)
			client := newFakeEmailMailingListClient(current)
			resource := &emailMailingListResource{client: client}
			state := testEmailMailingListModel(t, current)
			plan := state
			plan.Advertised = types.BoolValue(true)
			config := plan
			test.configure(&plan, &config)

			response := runEmailMailingListUpdate(
				t,
				resource,
				state,
				plan,
				config,
			)
			if !response.Diagnostics.HasError() {
				t.Fatal("Update() returned no validation error")
			}
			if !strings.Contains(
				fmt.Sprint(response.Diagnostics),
				test.wantDiagnostic,
			) {
				t.Fatalf(
					"Update() diagnostics = %v, want %q",
					response.Diagnostics,
					test.wantDiagnostic,
				)
			}
			if len(client.privacyCalls) != 0 {
				t.Fatalf(
					"privacy calls = %#v, want none",
					client.privacyCalls,
				)
			}
			if len(client.passwordCalls) != 0 {
				t.Fatalf(
					"password calls = %#v, want none",
					client.passwordCalls,
				)
			}
			if actual := mailingListPrivacy(*client.current); actual != previous {
				t.Fatalf(
					"remote privacy = %#v, want %#v",
					actual,
					previous,
				)
			}
		})
	}
}

func TestEmailMailingListUpdateRestoresPrivacyAfterPasswordFailure(
	t *testing.T,
) {
	t.Parallel()

	previous := testEmailMailingListPrivate()
	desired := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	current := testEmailMailingList(previous)
	client := newFakeEmailMailingListClient(current)
	client.passwordErr = errors.New("password response lost")
	resource := &emailMailingListResource{client: client}
	state := testEmailMailingListModel(t, current)
	plan := state
	plan.PasswordVersion = types.Int64Value(2)
	plan.Advertised = types.BoolValue(desired.Advertised)
	plan.ArchivePrivate = types.BoolValue(desired.ArchivePrivate)
	plan.SubscribePolicy = types.Int64Value(desired.SubscribePolicy)
	config := plan
	config.Password = types.StringValue("new-password")

	response := runEmailMailingListUpdate(t, resource, state, plan, config)
	if !response.Diagnostics.HasError() {
		t.Fatal("Update() returned no password mutation error")
	}
	if len(client.passwordCalls) != 1 {
		t.Fatalf("password calls = %d, want 1", len(client.passwordCalls))
	}
	if len(client.privacyCalls) != 2 {
		t.Fatalf(
			"privacy calls = %#v, want desired update and rollback",
			client.privacyCalls,
		)
	}
	if client.privacyCalls[0] != desired ||
		client.privacyCalls[1] != previous {
		t.Fatalf(
			"privacy calls = %#v, want [%#v %#v]",
			client.privacyCalls,
			desired,
			previous,
		)
	}
	if actual := mailingListPrivacy(*client.current); actual != previous {
		t.Fatalf("remote privacy = %#v, want %#v", actual, previous)
	}
	assertEmailMailingListNoReplacement(t, client)
}

func TestEmailMailingListUpdateRefusesRemotePrivacyDrift(t *testing.T) {
	t.Parallel()

	original := testEmailMailingList(testEmailMailingListPrivate())
	state := testEmailMailingListModel(t, original)
	remote := original
	remote.Advertised = !remote.Advertised
	client := newFakeEmailMailingListClient(remote)
	resource := &emailMailingListResource{client: client}
	plan := state
	plan.ArchivePrivate = types.BoolValue(!state.ArchivePrivate.ValueBool())
	config := plan
	config.Password = types.StringValue("password")

	response := runEmailMailingListUpdate(
		t,
		resource,
		state,
		plan,
		config,
	)
	assertUpdateDriftRefused(t, response.Diagnostics)
	if len(client.privacyCalls) != 0 ||
		len(client.passwordCalls) != 0 {
		t.Fatalf(
			"mutation calls: privacy=%#v password=%#v, want none",
			client.privacyCalls,
			client.passwordCalls,
		)
	}
}

func TestEmailMailingListUpdateRedactsPasswordFromDiagnostics(t *testing.T) {
	t.Parallel()

	const password = "Tf9!reflected-secret"
	current := testEmailMailingList(testEmailMailingListPrivate())
	client := newFakeEmailMailingListClient(current)
	client.passwordErr = &cpanelapi.APIError{
		API:      "UAPI",
		Module:   "Email",
		Function: "change_mlist_password",
		Messages: []string{"rejected password " + password},
	}
	resource := &emailMailingListResource{client: client}
	state := testEmailMailingListModel(t, current)
	plan := state
	plan.PasswordVersion = types.Int64Value(2)
	config := plan
	config.Password = types.StringValue(password)

	response := runEmailMailingListUpdate(t, resource, state, plan, config)
	if !response.Diagnostics.HasError() {
		t.Fatal("Update() returned no password mutation error")
	}
	detail := fmt.Sprint(response.Diagnostics)
	if strings.Contains(detail, password) {
		t.Fatalf("Update() diagnostics expose the password: %s", detail)
	}
	if !strings.Contains(detail, "rejected the mailing list password update") {
		t.Fatalf("Update() diagnostics = %s, want sanitized rejection", detail)
	}
}

func TestEmailMailingListSetPrivacyAcceptsVerifiedAmbiguousMutation(
	t *testing.T,
) {
	t.Parallel()

	current := testEmailMailingList(testEmailMailingListPrivate())
	desired := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 2,
	}
	client := newFakeEmailMailingListClient(current)
	client.privacyErr = errors.New("connection closed after request")
	client.privacyMutatesOnError = true
	resource := &emailMailingListResource{client: client}

	actual, err := resource.setPrivacyAndVerify(
		t.Context(),
		current.Address,
		"example.test",
		current.ID,
		desired,
	)
	if err != nil {
		t.Fatalf("setPrivacyAndVerify() error: %v", err)
	}
	if actual == nil || mailingListPrivacy(*actual) != desired {
		t.Fatalf("setPrivacyAndVerify() = %#v, want %#v", actual, desired)
	}
	assertEmailMailingListNoReplacement(t, client)
}

func TestEmailMailingListRollbackUpdatedPrivacyPreservesConcurrentChange(
	t *testing.T,
) {
	t.Parallel()

	previous := testEmailMailingListPrivate()
	attempted := cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
	concurrent := cpanelmail.MailingListPrivacyOptions{
		Advertised:      false,
		ArchivePrivate:  false,
		SubscribePolicy: 2,
	}
	current := testEmailMailingList(concurrent)
	client := newFakeEmailMailingListClient(current)
	resource := &emailMailingListResource{client: client}

	err := resource.rollbackUpdatedPrivacy(
		t.Context(),
		current.Address,
		"example.test",
		current.ID,
		previous,
		attempted,
	)
	if err == nil {
		t.Fatal("rollbackUpdatedPrivacy() returned no error")
	}
	if len(client.privacyCalls) != 0 {
		t.Fatalf("privacy calls = %#v, want none", client.privacyCalls)
	}
	if actual := mailingListPrivacy(*client.current); actual != concurrent {
		t.Fatalf("remote privacy = %#v, want %#v", actual, concurrent)
	}
}

func TestEmailMailingListRollbackCreatedReportsVerificationFailure(
	t *testing.T,
) {
	t.Parallel()

	current := testEmailMailingList(testEmailMailingListPrivate())
	client := newFakeEmailMailingListClient(current)
	client.applyDelete = false
	resource := &emailMailingListResource{client: client}

	err := resource.rollbackCreatedMailingList(
		t.Context(),
		current.Address,
		"example.test",
		current.ID,
		mailingListPrivacy(current),
	)
	if err == nil {
		t.Fatal("rollbackCreatedMailingList() returned no error")
	}
	if strings.Contains(err.Error(), "%!w(<nil>)") {
		t.Fatalf("rollback error wraps nil mutation: %v", err)
	}
	if client.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", client.deleteCalls)
	}
}

func TestEmailMailingListCreateStartsPrivate(t *testing.T) {
	t.Parallel()

	client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
	client.current = nil
	resource := &emailMailingListResource{client: client}

	actual, rollbackAllowed, err := resource.createAndVerify(
		t.Context(),
		"terraform@example.test",
		"terraform",
		"example.test",
		"password",
	)
	if err != nil {
		t.Fatalf("createAndVerify() error: %v", err)
	}
	if !rollbackAllowed {
		t.Fatal("createAndVerify() disallowed rollback")
	}
	if actual == nil || !actual.Private {
		t.Fatalf("createAndVerify() = %#v, want private list", actual)
	}
	if len(client.createPrivateCalls) != 1 ||
		!client.createPrivateCalls[0] {
		t.Fatalf(
			"create private calls = %#v, want [true]",
			client.createPrivateCalls,
		)
	}
}

func TestEmailMailingListCreateRedactsPasswordFromErrors(t *testing.T) {
	t.Parallel()

	const password = "Tf9!reflected-secret"
	tests := map[string]error{
		"api rejection": &cpanelapi.APIError{
			API:      "UAPI",
			Module:   "Email",
			Function: "add_list",
			Messages: []string{"rejected password " + password},
		},
		"ambiguous transport": errors.New(
			"connection failed after sending " + password,
		),
	}

	for name, mutationErr := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
			client.current = nil
			client.createErr = mutationErr
			resource := &emailMailingListResource{client: client}

			_, _, err := resource.createAndVerify(
				t.Context(),
				"terraform@example.test",
				"terraform",
				"example.test",
				password,
			)
			if err == nil {
				t.Fatal("createAndVerify() returned no error")
			}
			if strings.Contains(err.Error(), password) {
				t.Fatalf("createAndVerify() exposes the password: %v", err)
			}
		})
	}
}

func TestEmailMailingListCreateReadsWriteOnlyPasswordFromConfig(t *testing.T) {
	t.Parallel()

	client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
	client.current = nil
	resource := &emailMailingListResource{client: client}
	privacy := testEmailMailingListPrivate()
	plan := EmailMailingListResourceModel{
		Address:         types.StringValue("terraform@example.test"),
		Password:        types.StringNull(),
		PasswordVersion: types.Int64Value(1),
		DeleteOnDestroy: types.BoolValue(true),
		Advertised:      types.BoolValue(privacy.Advertised),
		ArchivePrivate:  types.BoolValue(privacy.ArchivePrivate),
		SubscribePolicy: types.Int64Value(privacy.SubscribePolicy),
		Private:         types.BoolUnknown(),
		ListID:          types.StringUnknown(),
		Administrators:  types.SetUnknown(types.StringType),
		HumanDiskUsed:   types.StringUnknown(),
	}
	config := plan
	config.Password = types.StringValue("write-only-password")

	response := runEmailMailingListCreate(t, resource, plan, config)
	if response.Diagnostics.HasError() {
		t.Fatalf("Create() diagnostics: %v", response.Diagnostics)
	}
	if len(client.createPasswordCalls) != 1 ||
		client.createPasswordCalls[0] != "write-only-password" {
		t.Fatalf(
			"create password calls = %#v, want [write-only-password]",
			client.createPasswordCalls,
		)
	}

	var created EmailMailingListResourceModel
	diagnostics := response.State.Get(t.Context(), &created)
	if diagnostics.HasError() {
		t.Fatalf("State.Get() diagnostics: %v", diagnostics)
	}
	if !created.Password.IsNull() {
		t.Fatal("write-only password was stored in resource state")
	}
	if created.PasswordVersion.ValueInt64() != 1 {
		t.Fatalf(
			"password_version = %d, want 1",
			created.PasswordVersion.ValueInt64(),
		)
	}
}

func TestEmailMailingListDeleteRequiresExplicitOptIn(t *testing.T) {
	t.Parallel()

	for _, deleteOnDestroy := range []bool{false, true} {
		deleteOnDestroy := deleteOnDestroy
		t.Run(fmt.Sprintf("delete_on_destroy=%t", deleteOnDestroy), func(t *testing.T) {
			t.Parallel()

			current := testEmailMailingList(testEmailMailingListPrivate())
			client := newFakeEmailMailingListClient(current)
			resource := &emailMailingListResource{client: client}
			state := testEmailMailingListModel(t, current)
			state.DeleteOnDestroy = types.BoolValue(deleteOnDestroy)

			response := runEmailMailingListDelete(t, resource, state)
			if response.Diagnostics.HasError() {
				t.Fatalf("Delete() diagnostics: %v", response.Diagnostics)
			}
			wantDeleteCalls := 0
			if deleteOnDestroy {
				wantDeleteCalls = 1
			}
			if client.deleteCalls != wantDeleteCalls {
				t.Fatalf(
					"delete calls = %d, want %d",
					client.deleteCalls,
					wantDeleteCalls,
				)
			}
			if deleteOnDestroy && client.current != nil {
				t.Fatal("mailing list still exists after opted-in deletion")
			}
			if !deleteOnDestroy && client.current == nil {
				t.Fatal("mailing list was deleted without explicit opt-in")
			}
		})
	}
}

func TestEmailMailingListCreateDoesNotAdoptAmbiguousResult(t *testing.T) {
	t.Parallel()

	client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
	client.current = nil
	client.createErr = errors.New("connection closed after request")
	client.createMutatesOnError = true
	resource := &emailMailingListResource{client: client}

	actual, rollbackAllowed, err := resource.createAndVerify(
		t.Context(),
		"terraform@example.test",
		"terraform",
		"example.test",
		"password",
	)
	if err == nil {
		t.Fatal("createAndVerify() returned no error")
	}
	if actual == nil {
		t.Fatal("createAndVerify() did not return the observed list")
	}
	if rollbackAllowed {
		t.Fatal("createAndVerify() allowed rollback after ambiguous creation")
	}
	if !strings.Contains(err.Error(), "will not adopt or delete") {
		t.Fatalf("createAndVerify() error = %v", err)
	}
	if client.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", client.deleteCalls)
	}
}

func TestEmailMailingListCreateDoesNotAdoptRejectedResult(t *testing.T) {
	t.Parallel()

	client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
	client.current = nil
	client.createErr = &cpanelapi.APIError{
		API:      "UAPI",
		Module:   "Email",
		Function: "add_list",
		Messages: []string{"request rejected"},
	}
	client.createMutatesOnError = true
	resource := &emailMailingListResource{client: client}

	actual, rollbackAllowed, err := resource.createAndVerify(
		t.Context(),
		"terraform@example.test",
		"terraform",
		"example.test",
		"password",
	)
	if err == nil {
		t.Fatal("createAndVerify() returned no error")
	}
	if actual == nil {
		t.Fatal("createAndVerify() did not return the observed list")
	}
	if rollbackAllowed {
		t.Fatal("createAndVerify() allowed rollback after rejected creation")
	}
	if !strings.Contains(err.Error(), "refusing to adopt") {
		t.Fatalf("createAndVerify() error = %v", err)
	}
	if client.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", client.deleteCalls)
	}
}

func TestEmailMailingListCreateAllowsRollbackAfterConfirmedValidationFailure(
	t *testing.T,
) {
	t.Parallel()

	client := newFakeEmailMailingListClient(cpanelmail.MailingList{})
	client.current = nil
	public := testEmailMailingList(cpanelmail.MailingListPrivacyOptions{
		Advertised:      true,
		ArchivePrivate:  false,
		SubscribePolicy: 1,
	})
	client.createdOverride = &public
	resource := &emailMailingListResource{client: client}

	actual, rollbackAllowed, err := resource.createAndVerify(
		t.Context(),
		"terraform@example.test",
		"terraform",
		"example.test",
		"password",
	)
	if err == nil {
		t.Fatal("createAndVerify() returned no validation error")
	}
	if actual == nil {
		t.Fatal("createAndVerify() did not return the observed list")
	}
	if !rollbackAllowed {
		t.Fatal("createAndVerify() disallowed rollback after confirmed creation")
	}
}

func TestEmailMailingListMutationErrorClassification(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want bool
	}{
		"api error": {
			err: &cpanelapi.APIError{
				API:      "UAPI",
				Module:   "Email",
				Function: "add_list",
				Messages: []string{"rejected"},
			},
			want: true,
		},
		"wrapped 400": {
			err:  fmt.Errorf("request: %w", &cpanelapi.HTTPError{StatusCode: 400}),
			want: true,
		},
		"wrapped 499": {
			err:  fmt.Errorf("request: %w", &cpanelapi.HTTPError{StatusCode: 499}),
			want: true,
		},
		"wrapped 500": {
			err:  fmt.Errorf("request: %w", &cpanelapi.HTTPError{StatusCode: 500}),
			want: false,
		},
		"transport": {
			err:  errors.New("connection reset"),
			want: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := mailingListMutationErrorIsDeterministic(test.err); got != test.want {
				t.Fatalf(
					"mailingListMutationErrorIsDeterministic() = %t, want %t",
					got,
					test.want,
				)
			}
		})
	}
}

func runEmailMailingListUpdate(
	t *testing.T,
	resource *emailMailingListResource,
	stateModel EmailMailingListResourceModel,
	planModel EmailMailingListResourceModel,
	configModel EmailMailingListResourceModel,
) *frameworkresource.UpdateResponse {
	t.Helper()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewEmailMailingListResource().Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(ctx, &stateModel)
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics = plan.Set(ctx, &planModel)
	if diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	configValue := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics = configValue.Set(ctx, &configModel)
	if diagnostics.HasError() {
		t.Fatalf("Config.Set() diagnostics: %v", diagnostics)
	}
	config := tfsdk.Config{
		Raw:    configValue.Raw,
		Schema: schemaResponse.Schema,
	}
	response := &frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	resource.Update(
		ctx,
		frameworkresource.UpdateRequest{
			State:  state,
			Plan:   plan,
			Config: config,
		},
		response,
	)

	return response
}

func runEmailMailingListCreate(
	t *testing.T,
	resource *emailMailingListResource,
	planModel EmailMailingListResourceModel,
	configModel EmailMailingListResourceModel,
) *frameworkresource.CreateResponse {
	t.Helper()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewEmailMailingListResource().Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics := plan.Set(ctx, &planModel)
	if diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}
	configValue := tfsdk.Plan{Schema: schemaResponse.Schema}
	diagnostics = configValue.Set(ctx, &configModel)
	if diagnostics.HasError() {
		t.Fatalf("Config.Set() diagnostics: %v", diagnostics)
	}
	config := tfsdk.Config{
		Raw:    configValue.Raw,
		Schema: schemaResponse.Schema,
	}
	response := &frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	resource.Create(
		ctx,
		frameworkresource.CreateRequest{
			Plan:   plan,
			Config: config,
		},
		response,
	)

	return response
}

func runEmailMailingListDelete(
	t *testing.T,
	resource *emailMailingListResource,
	stateModel EmailMailingListResourceModel,
) *frameworkresource.DeleteResponse {
	t.Helper()

	ctx := t.Context()
	schemaResponse := &frameworkresource.SchemaResponse{}
	NewEmailMailingListResource().Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(ctx, &stateModel)
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	response := &frameworkresource.DeleteResponse{}
	resource.Delete(
		ctx,
		frameworkresource.DeleteRequest{State: state},
		response,
	)

	return response
}

func testEmailMailingListModel(
	t *testing.T,
	mailingList cpanelmail.MailingList,
) EmailMailingListResourceModel {
	t.Helper()

	administrators, diagnostics := types.SetValueFrom(
		t.Context(),
		types.StringType,
		mailingList.Administrators,
	)
	if diagnostics.HasError() {
		t.Fatalf("SetValueFrom() diagnostics: %v", diagnostics)
	}

	return EmailMailingListResourceModel{
		Address:         types.StringValue(mailingList.Address),
		Password:        types.StringNull(),
		PasswordVersion: types.Int64Value(1),
		DeleteOnDestroy: types.BoolValue(true),
		Advertised:      types.BoolValue(mailingList.Advertised),
		ArchivePrivate:  types.BoolValue(mailingList.ArchivePrivate),
		SubscribePolicy: types.Int64Value(mailingList.SubscribePolicy),
		Private:         types.BoolValue(mailingList.Private),
		ListID:          types.StringValue(mailingList.ID),
		Administrators:  administrators,
		HumanDiskUsed:   types.StringValue(mailingList.HumanDiskUsed),
	}
}

func testEmailMailingListPrivate() cpanelmail.MailingListPrivacyOptions {
	return cpanelmail.MailingListPrivacyOptions{
		ArchivePrivate:  true,
		SubscribePolicy: 3,
	}
}

func testEmailMailingList(
	privacy cpanelmail.MailingListPrivacyOptions,
) cpanelmail.MailingList {
	private := !privacy.Advertised &&
		privacy.ArchivePrivate &&
		(privacy.SubscribePolicy == 2 || privacy.SubscribePolicy == 3)

	return cpanelmail.MailingList{
		Address:         "terraform@example.test",
		Private:         private,
		ID:              "terraform_example.test",
		Administrators:  []string{"owner@example.test"},
		Advertised:      privacy.Advertised,
		ArchivePrivate:  privacy.ArchivePrivate,
		SubscribePolicy: privacy.SubscribePolicy,
		HumanDiskUsed:   "0 bytes",
	}
}

func assertEmailMailingListNoReplacement(
	t *testing.T,
	client *fakeEmailMailingListClient,
) {
	t.Helper()

	if client.createCalls != 0 || client.deleteCalls != 0 {
		t.Fatalf(
			"replacement calls: create=%d delete=%d",
			client.createCalls,
			client.deleteCalls,
		)
	}
}

type fakeEmailMailingListClient struct {
	current *cpanelmail.MailingList

	passwordErr           error
	privacyErr            error
	privacyMutatesOnError bool
	createErr             error
	createMutatesOnError  bool
	createdOverride       *cpanelmail.MailingList
	deleteErr             error
	applyDelete           bool

	passwordCalls       []string
	privacyCalls        []cpanelmail.MailingListPrivacyOptions
	createPasswordCalls []string
	createPrivateCalls  []bool
	createCalls         int
	deleteCalls         int
}

func newFakeEmailMailingListClient(
	current cpanelmail.MailingList,
) *fakeEmailMailingListClient {
	currentCopy := current

	return &fakeEmailMailingListClient{
		current:     &currentCopy,
		applyDelete: true,
	}
}

func (c *fakeEmailMailingListClient) ChangeMailingListPassword(
	_ context.Context,
	_ string,
	password string,
) error {
	c.passwordCalls = append(c.passwordCalls, password)

	return c.passwordErr
}

func (c *fakeEmailMailingListClient) CreateMailingList(
	_ context.Context,
	user string,
	domain string,
	password string,
	private bool,
) error {
	c.createCalls++
	c.createPasswordCalls = append(c.createPasswordCalls, password)
	c.createPrivateCalls = append(c.createPrivateCalls, private)
	if c.createErr == nil || c.createMutatesOnError {
		if c.createdOverride != nil {
			created := *c.createdOverride
			created.Address = user + "@" + domain
			c.current = &created

			return c.createErr
		}

		privacy := cpanelmail.MailingListPrivacyOptions{
			ArchivePrivate:  private,
			SubscribePolicy: 1,
		}
		if private {
			privacy.SubscribePolicy = 3
		}
		created := testEmailMailingList(privacy)
		created.Address = user + "@" + domain
		c.current = &created
	}

	return c.createErr
}

func (c *fakeEmailMailingListClient) DeleteMailingList(
	context.Context,
	string,
) error {
	c.deleteCalls++
	if c.applyDelete {
		c.current = nil
	}

	return c.deleteErr
}

func (c *fakeEmailMailingListClient) GetMailingList(
	_ context.Context,
	address string,
	_ string,
) (*cpanelmail.MailingList, error) {
	if c.current == nil || c.current.Address != address {
		return nil, nil
	}
	currentCopy := *c.current
	currentCopy.Administrators = append(
		[]string(nil),
		c.current.Administrators...,
	)

	return &currentCopy, nil
}

func (c *fakeEmailMailingListClient) ListMailDomains(
	context.Context,
) ([]string, error) {
	return []string{"example.test"}, nil
}

func (c *fakeEmailMailingListClient) LockMailingList(string) func() {
	return func() {}
}

func (c *fakeEmailMailingListClient) SetMailingListPrivacyOptions(
	_ context.Context,
	_ string,
	privacy cpanelmail.MailingListPrivacyOptions,
) error {
	c.privacyCalls = append(c.privacyCalls, privacy)
	if c.current != nil &&
		(c.privacyErr == nil || c.privacyMutatesOnError) {
		c.current.Advertised = privacy.Advertised
		c.current.ArchivePrivate = privacy.ArchivePrivate
		c.current.SubscribePolicy = privacy.SubscribePolicy
		c.current.Private = !privacy.Advertised &&
			privacy.ArchivePrivate &&
			(privacy.SubscribePolicy == 2 ||
				privacy.SubscribePolicy == 3)
	}

	return c.privacyErr
}
