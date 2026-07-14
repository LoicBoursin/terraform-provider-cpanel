package provider

import (
	"encoding/json"
	"testing"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailAccountSuspensionModelMapping(t *testing.T) {
	t.Parallel()

	account := cpanelmail.Account{
		Email:                "terraform@example.test",
		HasSuspendedRaw:      json.RawMessage(`1`),
		HoldOutgoingRaw:      json.RawMessage(`0`),
		SuspendedIncomingRaw: json.RawMessage(`0`),
		SuspendedLoginRaw:    json.RawMessage(`1`),
		SuspendedOutgoingRaw: json.RawMessage(`1`),
	}

	model := EmailAccountSuspensionResourceModel{}
	if err := applyEmailAccountSuspensionsToResourceModel(
		&model,
		account,
	); err != nil {
		t.Fatalf("applyEmailAccountSuspensionsToResourceModel() error: %v", err)
	}
	if model.Email.ValueString() != account.Email ||
		!model.LoginSuspended.ValueBool() ||
		model.IncomingSuspended.ValueBool() ||
		!model.OutgoingSuspended.ValueBool() ||
		model.OutgoingHeld.ValueBool() ||
		!model.HasSuspended.ValueBool() {
		t.Fatalf("resource model = %#v", model)
	}

	dataSourceModel, err := emailAccountSuspensionsToDataSourceModel(account)
	if err != nil {
		t.Fatalf("emailAccountSuspensionsToDataSourceModel() error: %v", err)
	}
	if dataSourceModel.Email.ValueString() != account.Email ||
		!dataSourceModel.LoginSuspended.ValueBool() ||
		!dataSourceModel.HasSuspended.ValueBool() {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
