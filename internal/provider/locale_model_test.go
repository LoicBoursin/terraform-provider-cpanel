package provider

import (
	"testing"

	cpanellocale "terraform-provider-cpanel/internal/cpanel/locale"
)

func TestLocaleModelMapping(t *testing.T) {
	t.Parallel()

	value := cpanellocale.Locale{
		Code:      "fr",
		Direction: cpanellocale.DirectionLeftToRight,
		Encoding:  "utf-8",
		LocalName: "français",
		Name:      "French",
	}

	resourceModel := LocaleResourceModel{}
	applyLocaleToResourceModel(&resourceModel, value)
	if resourceModel.Account.ValueString() != cpanellocale.AccountIdentity ||
		resourceModel.Locale.ValueString() != value.Code ||
		resourceModel.Name.ValueString() != value.Name ||
		resourceModel.LocalName.ValueString() != value.LocalName ||
		resourceModel.Direction.ValueString() != value.Direction ||
		resourceModel.Encoding.ValueString() != value.Encoding {
		t.Fatalf("resource model = %#v", resourceModel)
	}

	dataSourceModel := localeToDataSourceModel(value)
	if dataSourceModel.Account.ValueString() != cpanellocale.AccountIdentity ||
		dataSourceModel.Locale.ValueString() != value.Code ||
		dataSourceModel.Name.ValueString() != value.Name ||
		dataSourceModel.LocalName.ValueString() != value.LocalName ||
		dataSourceModel.Direction.ValueString() != value.Direction ||
		dataSourceModel.Encoding.ValueString() != value.Encoding {
		t.Fatalf("data source model = %#v", dataSourceModel)
	}
}
