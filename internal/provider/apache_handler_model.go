package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

type ApacheHandlerResourceModel struct {
	Extension types.String `tfsdk:"extension"`
	Handler   types.String `tfsdk:"handler"`
	Origin    types.String `tfsdk:"origin"`
}

type ApacheHandlerDataSourceModel struct {
	Extension types.String `tfsdk:"extension"`
	Handler   types.String `tfsdk:"handler"`
	Origin    types.String `tfsdk:"origin"`
}

func applyApacheHandlerToResourceModel(
	model *ApacheHandlerResourceModel,
	apiHandler apachehandler.Handler,
) {
	model.Extension = types.StringValue(apiHandler.Extension)
	model.Handler = types.StringValue(apiHandler.Handler)
	model.Origin = types.StringValue(apiHandler.Origin)
}

func apacheHandlerToDataSourceModel(
	apiHandler apachehandler.Handler,
) *ApacheHandlerDataSourceModel {
	return &ApacheHandlerDataSourceModel{
		Extension: types.StringValue(apiHandler.Extension),
		Handler:   types.StringValue(apiHandler.Handler),
		Origin:    types.StringValue(apiHandler.Origin),
	}
}

func apacheHandlerDefinitionFromResourceModel(
	model ApacheHandlerResourceModel,
) apachehandler.Definition {
	return apachehandler.Definition{
		Extension: model.Extension.ValueString(),
		Handler:   model.Handler.ValueString(),
	}
}

func apacheHandlerDefinitionFromAPI(
	apiHandler apachehandler.Handler,
) apachehandler.Definition {
	return apachehandler.Definition{
		Extension: apiHandler.Extension,
		Handler:   apiHandler.Handler,
	}
}
