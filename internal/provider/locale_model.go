package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

type LocaleResourceModel struct {
	Account       types.String `tfsdk:"account"`
	Locale        types.String `tfsdk:"locale"`
	RestoreLocale types.String `tfsdk:"restore_locale"`
	Name          types.String `tfsdk:"name"`
	LocalName     types.String `tfsdk:"local_name"`
	Direction     types.String `tfsdk:"direction"`
	Encoding      types.String `tfsdk:"encoding"`
}

type LocaleDataSourceModel struct {
	Account   types.String `tfsdk:"account"`
	Locale    types.String `tfsdk:"locale"`
	Name      types.String `tfsdk:"name"`
	LocalName types.String `tfsdk:"local_name"`
	Direction types.String `tfsdk:"direction"`
	Encoding  types.String `tfsdk:"encoding"`
}

func applyLocaleToResourceModel(
	model *LocaleResourceModel,
	value cpanellocale.Locale,
) {
	model.Account = types.StringValue(cpanellocale.AccountIdentity)
	model.Locale = types.StringValue(value.Code)
	model.Name = types.StringValue(value.Name)
	model.LocalName = types.StringValue(value.LocalName)
	model.Direction = types.StringValue(value.Direction)
	model.Encoding = types.StringValue(value.Encoding)
}

func localeToDataSourceModel(
	value cpanellocale.Locale,
) *LocaleDataSourceModel {
	return &LocaleDataSourceModel{
		Account:   types.StringValue(cpanellocale.AccountIdentity),
		Locale:    types.StringValue(value.Code),
		Name:      types.StringValue(value.Name),
		LocalName: types.StringValue(value.LocalName),
		Direction: types.StringValue(value.Direction),
		Encoding:  types.StringValue(value.Encoding),
	}
}
