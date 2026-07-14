package provider

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelredirect "terraform-provider-cpanel/internal/cpanel/redirect"
)

type RedirectResourceModel struct {
	Domain       types.String `tfsdk:"domain"`
	Source       types.String `tfsdk:"source"`
	Destination  types.String `tfsdk:"destination"`
	Type         types.String `tfsdk:"type"`
	WWWMode      types.String `tfsdk:"www_mode"`
	Wildcard     types.Bool   `tfsdk:"wildcard"`
	StatusCode   types.Int64  `tfsdk:"status_code"`
	DocumentRoot types.String `tfsdk:"document_root"`
	Kind         types.String `tfsdk:"kind"`
}

type RedirectDataSourceModel struct {
	Domain       types.String `tfsdk:"domain"`
	Source       types.String `tfsdk:"source"`
	Destination  types.String `tfsdk:"destination"`
	Type         types.String `tfsdk:"type"`
	WWWMode      types.String `tfsdk:"www_mode"`
	Wildcard     types.Bool   `tfsdk:"wildcard"`
	StatusCode   types.Int64  `tfsdk:"status_code"`
	DocumentRoot types.String `tfsdk:"document_root"`
	Kind         types.String `tfsdk:"kind"`
}

func applyRedirectToResourceModel(
	model *RedirectResourceModel,
	redirect cpanelredirect.Redirect,
) diag.Diagnostics {
	statusCode, diagnostics := redirectStatusCode(redirect)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Domain = types.StringValue(redirect.Domain)
	model.Source = types.StringValue(redirect.Source)
	model.Destination = types.StringValue(redirect.Destination)
	model.Type = types.StringValue(redirect.Type)
	model.WWWMode = types.StringValue(redirectWWWMode(redirect))
	model.Wildcard = types.BoolValue(redirect.Wildcard == 1)
	model.StatusCode = statusCode
	model.DocumentRoot = types.StringValue(redirect.DocumentRoot)
	model.Kind = types.StringValue(redirect.Kind)

	return diagnostics
}

func redirectToDataSourceModel(
	redirect cpanelredirect.Redirect,
) (*RedirectDataSourceModel, diag.Diagnostics) {
	statusCode, diagnostics := redirectStatusCode(redirect)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &RedirectDataSourceModel{
		Domain:       types.StringValue(redirect.Domain),
		Source:       types.StringValue(redirect.Source),
		Destination:  types.StringValue(redirect.Destination),
		Type:         types.StringValue(redirect.Type),
		WWWMode:      types.StringValue(redirectWWWMode(redirect)),
		Wildcard:     types.BoolValue(redirect.Wildcard == 1),
		StatusCode:   statusCode,
		DocumentRoot: types.StringValue(redirect.DocumentRoot),
		Kind:         types.StringValue(redirect.Kind),
	}, diagnostics
}

func redirectDefinitionFromResourceModel(
	model RedirectResourceModel,
) cpanelredirect.Definition {
	return cpanelredirect.Definition{
		Domain:      model.Domain.ValueString(),
		Source:      model.Source.ValueString(),
		Destination: model.Destination.ValueString(),
		Type:        model.Type.ValueString(),
		WWWMode:     model.WWWMode.ValueString(),
		Wildcard:    model.Wildcard.ValueBool(),
	}
}

func redirectDefinitionFromAPI(
	redirect cpanelredirect.Redirect,
) cpanelredirect.Definition {
	return cpanelredirect.Definition{
		Domain:      redirect.Domain,
		Source:      redirect.Source,
		Destination: redirect.Destination,
		Type:        redirect.Type,
		WWWMode:     redirectWWWMode(redirect),
		Wildcard:    redirect.Wildcard == 1,
	}
}

func redirectWWWMode(redirect cpanelredirect.Redirect) string {
	if redirect.MatchWWW == 0 {
		return cpanelredirect.WWWModeWithout
	}

	return cpanelredirect.WWWModeBoth
}

func redirectStatusCode(
	redirect cpanelredirect.Redirect,
) (types.Int64, diag.Diagnostics) {
	var diagnostics diag.Diagnostics

	statusCode, err := strconv.ParseInt(redirect.StatusCode, 10, 64)
	if err != nil {
		diagnostics.AddError(
			"Unable to decode redirect status code",
			fmt.Sprintf(
				"cPanel returned invalid status code %q for redirect %s%s.",
				redirect.StatusCode,
				redirect.Domain,
				redirect.Source,
			),
		)

		return types.Int64Unknown(), diagnostics
	}

	return types.Int64Value(statusCode), diagnostics
}
