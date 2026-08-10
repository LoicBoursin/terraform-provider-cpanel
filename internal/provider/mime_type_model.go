package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

type MIMETypeResourceModel struct {
	Type       types.String `tfsdk:"type"`
	Extensions types.Set    `tfsdk:"extensions"`
	Origin     types.String `tfsdk:"origin"`
}

type MIMETypeDataSourceModel struct {
	Type       types.String `tfsdk:"type"`
	Extensions types.Set    `tfsdk:"extensions"`
	Origin     types.String `tfsdk:"origin"`
}

func applyMIMETypeToResourceModel(
	ctx context.Context,
	model *MIMETypeResourceModel,
	apiMIMEType mimetype.MIMEType,
) diag.Diagnostics {
	extensions, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		apiMIMEType.Extensions(),
	)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Type = types.StringValue(apiMIMEType.Type)
	model.Extensions = extensions
	model.Origin = types.StringValue(apiMIMEType.Origin)

	return diagnostics
}

func mimeTypeToDataSourceModel(
	ctx context.Context,
	apiMIMEType mimetype.MIMEType,
) (*MIMETypeDataSourceModel, diag.Diagnostics) {
	extensions, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		apiMIMEType.Extensions(),
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &MIMETypeDataSourceModel{
		Type:       types.StringValue(apiMIMEType.Type),
		Extensions: extensions,
		Origin:     types.StringValue(apiMIMEType.Origin),
	}, diagnostics
}

func mimeTypeDefinitionFromResourceModel(
	ctx context.Context,
	model MIMETypeResourceModel,
) (mimetype.Definition, diag.Diagnostics) {
	var extensions []string
	diagnostics := model.Extensions.ElementsAs(ctx, &extensions, false)

	return mimetype.Definition{
		Type:       model.Type.ValueString(),
		Extensions: extensions,
	}.Sorted(), diagnostics
}

func mimeTypeDefinitionFromAPI(
	apiMIMEType mimetype.MIMEType,
) mimetype.Definition {
	return mimetype.Definition{
		Type:       apiMIMEType.Type,
		Extensions: apiMIMEType.Extensions(),
	}.Sorted()
}
