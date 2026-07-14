package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

type EmailAccountSuspensionResourceModel struct {
	Email             types.String `tfsdk:"email"`
	LoginSuspended    types.Bool   `tfsdk:"login_suspended"`
	IncomingSuspended types.Bool   `tfsdk:"incoming_suspended"`
	OutgoingSuspended types.Bool   `tfsdk:"outgoing_suspended"`
	OutgoingHeld      types.Bool   `tfsdk:"outgoing_held"`
	HasSuspended      types.Bool   `tfsdk:"has_suspended"`
}

type EmailAccountSuspensionDataSourceModel struct {
	Email             types.String `tfsdk:"email"`
	LoginSuspended    types.Bool   `tfsdk:"login_suspended"`
	IncomingSuspended types.Bool   `tfsdk:"incoming_suspended"`
	OutgoingSuspended types.Bool   `tfsdk:"outgoing_suspended"`
	OutgoingHeld      types.Bool   `tfsdk:"outgoing_held"`
	HasSuspended      types.Bool   `tfsdk:"has_suspended"`
}

type emailAccountSuspensionDefinition struct {
	Email    string
	Statuses cpanelmail.AccountSuspensions
}

func applyEmailAccountSuspensionsToResourceModel(
	model *EmailAccountSuspensionResourceModel,
	account cpanelmail.Account,
) error {
	suspensions, err := account.Suspensions()
	if err != nil {
		return err
	}

	model.Email = types.StringValue(account.Email)
	model.LoginSuspended = types.BoolValue(suspensions.Login)
	model.IncomingSuspended = types.BoolValue(suspensions.Incoming)
	model.OutgoingSuspended = types.BoolValue(suspensions.Outgoing)
	model.OutgoingHeld = types.BoolValue(suspensions.OutgoingHeld)
	model.HasSuspended = types.BoolValue(suspensions.HasSuspended)

	return nil
}

func emailAccountSuspensionsToDataSourceModel(
	account cpanelmail.Account,
) (*EmailAccountSuspensionDataSourceModel, error) {
	resourceModel := EmailAccountSuspensionResourceModel{}
	if err := applyEmailAccountSuspensionsToResourceModel(
		&resourceModel,
		account,
	); err != nil {
		return nil, err
	}

	return &EmailAccountSuspensionDataSourceModel{
		Email:             resourceModel.Email,
		LoginSuspended:    resourceModel.LoginSuspended,
		IncomingSuspended: resourceModel.IncomingSuspended,
		OutgoingSuspended: resourceModel.OutgoingSuspended,
		OutgoingHeld:      resourceModel.OutgoingHeld,
		HasSuspended:      resourceModel.HasSuspended,
	}, nil
}

func emailAccountSuspensionDefinitionFromResourceModel(
	model EmailAccountSuspensionResourceModel,
) emailAccountSuspensionDefinition {
	return emailAccountSuspensionDefinition{
		Email: model.Email.ValueString(),
		Statuses: cpanelmail.AccountSuspensions{
			Login:    model.LoginSuspended.ValueBool(),
			Incoming: model.IncomingSuspended.ValueBool(),
			Outgoing: model.OutgoingSuspended.ValueBool(),
		},
	}
}
