package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/apitoken"
)

type APITokenResourceModel struct {
	Name          types.String `tfsdk:"name"`
	ExpiresAt     types.Int64  `tfsdk:"expires_at"`
	Token         types.String `tfsdk:"token"`
	CreatedAt     types.Int64  `tfsdk:"created_at"`
	HasFullAccess types.Bool   `tfsdk:"has_full_access"`
	Features      types.Set    `tfsdk:"features"`
	WhitelistIPs  types.Set    `tfsdk:"whitelist_ips"`
}

type APITokenDataSourceModel struct {
	Name          types.String `tfsdk:"name"`
	ExpiresAt     types.Int64  `tfsdk:"expires_at"`
	CreatedAt     types.Int64  `tfsdk:"created_at"`
	HasFullAccess types.Bool   `tfsdk:"has_full_access"`
	Features      types.Set    `tfsdk:"features"`
	WhitelistIPs  types.Set    `tfsdk:"whitelist_ips"`
}

func applyAPITokenToResourceModel(
	ctx context.Context,
	model *APITokenResourceModel,
	token apitoken.Token,
) diag.Diagnostics {
	features, whitelistIPs, diagnostics := apiTokenSets(ctx, token)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Name = types.StringValue(token.Name)
	model.ExpiresAt = types.Int64Value(token.ExpiresAt.ValueOrZero())
	model.CreatedAt = types.Int64Value(token.CreateTime)
	model.HasFullAccess = types.BoolValue(token.HasFullAccess == 1)
	model.Features = features
	model.WhitelistIPs = whitelistIPs

	return diagnostics
}

func apiTokenToDataSourceModel(
	ctx context.Context,
	token apitoken.Token,
) (*APITokenDataSourceModel, diag.Diagnostics) {
	features, whitelistIPs, diagnostics := apiTokenSets(ctx, token)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &APITokenDataSourceModel{
		Name:          types.StringValue(token.Name),
		ExpiresAt:     types.Int64Value(token.ExpiresAt.ValueOrZero()),
		CreatedAt:     types.Int64Value(token.CreateTime),
		HasFullAccess: types.BoolValue(token.HasFullAccess == 1),
		Features:      features,
		WhitelistIPs:  whitelistIPs,
	}, diagnostics
}

func apiTokenSets(
	ctx context.Context,
	token apitoken.Token,
) (types.Set, types.Set, diag.Diagnostics) {
	features := token.Features
	if features == nil {
		features = []string{}
	}
	featureSet, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		features,
	)
	if diagnostics.HasError() {
		return types.Set{}, types.Set{}, diagnostics
	}

	whitelistIPs := token.WhitelistIPs
	if whitelistIPs == nil {
		whitelistIPs = []string{}
	}
	whitelistSet, whitelistDiagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		whitelistIPs,
	)
	diagnostics.Append(whitelistDiagnostics...)

	return featureSet, whitelistSet, diagnostics
}
