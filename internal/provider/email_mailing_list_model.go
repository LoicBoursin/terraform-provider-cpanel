package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

type EmailMailingListResourceModel struct {
	Address         types.String `tfsdk:"address"`
	Password        types.String `tfsdk:"password"`
	PasswordVersion types.Int64  `tfsdk:"password_version"`
	DeleteOnDestroy types.Bool   `tfsdk:"delete_on_destroy"`
	Advertised      types.Bool   `tfsdk:"advertised"`
	ArchivePrivate  types.Bool   `tfsdk:"archive_private"`
	SubscribePolicy types.Int64  `tfsdk:"subscribe_policy"`
	Private         types.Bool   `tfsdk:"private"`
	ListID          types.String `tfsdk:"list_id"`
	Administrators  types.Set    `tfsdk:"administrators"`
	HumanDiskUsed   types.String `tfsdk:"human_disk_used"`
}

type EmailMailingListDataSourceModel struct {
	Address         types.String `tfsdk:"address"`
	Private         types.Bool   `tfsdk:"private"`
	ListID          types.String `tfsdk:"list_id"`
	Administrators  types.Set    `tfsdk:"administrators"`
	Advertised      types.Bool   `tfsdk:"advertised"`
	ArchivePrivate  types.Bool   `tfsdk:"archive_private"`
	SubscribePolicy types.Int64  `tfsdk:"subscribe_policy"`
	HumanDiskUsed   types.String `tfsdk:"human_disk_used"`
}

func applyMailingListToResourceModel(
	ctx context.Context,
	model *EmailMailingListResourceModel,
	mailingList cpanelmail.MailingList,
) diag.Diagnostics {
	administrators, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		mailingList.Administrators,
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Address = types.StringValue(mailingList.Address)
	model.Private = types.BoolValue(mailingList.Private)
	model.ListID = types.StringValue(mailingList.ID)
	model.Administrators = administrators
	model.Advertised = types.BoolValue(mailingList.Advertised)
	model.ArchivePrivate = types.BoolValue(mailingList.ArchivePrivate)
	model.SubscribePolicy = types.Int64Value(mailingList.SubscribePolicy)
	model.HumanDiskUsed = types.StringValue(mailingList.HumanDiskUsed)

	return diagnostics
}

func mailingListDataSourceModel(
	ctx context.Context,
	mailingList cpanelmail.MailingList,
) (EmailMailingListDataSourceModel, diag.Diagnostics) {
	administrators, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		mailingList.Administrators,
	)
	if diagnostics.HasError() {
		return EmailMailingListDataSourceModel{}, diagnostics
	}

	return EmailMailingListDataSourceModel{
		Address:         types.StringValue(mailingList.Address),
		Private:         types.BoolValue(mailingList.Private),
		ListID:          types.StringValue(mailingList.ID),
		Administrators:  administrators,
		Advertised:      types.BoolValue(mailingList.Advertised),
		ArchivePrivate:  types.BoolValue(mailingList.ArchivePrivate),
		SubscribePolicy: types.Int64Value(mailingList.SubscribePolicy),
		HumanDiskUsed:   types.StringValue(mailingList.HumanDiskUsed),
	}, diagnostics
}
