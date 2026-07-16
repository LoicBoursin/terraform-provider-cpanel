package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

type EmailAddressInventoryDataSourceModel struct {
	Addresses types.List `tfsdk:"addresses"`
}

type EmailDomainInventoryDataSourceModel struct {
	Domains types.List `tfsdk:"domains"`
}

type EmailRoutingsDataSourceModel struct {
	Routings []EmailRoutingInventoryModel `tfsdk:"routings"`
}

type EmailRoutingInventoryModel struct {
	Domain types.String `tfsdk:"domain"`
	Mode   types.String `tfsdk:"mode"`
}

type EmailDomainForwardersDataSourceModel struct {
	Forwarders []EmailDomainForwarderInventoryModel `tfsdk:"forwarders"`
}

type EmailDomainForwarderInventoryModel struct {
	Domain      types.String `tfsdk:"domain"`
	Destination types.String `tfsdk:"destination"`
}

func emailInventoryStringList(
	ctx context.Context,
	values []string,
) (types.List, diag.Diagnostics) {
	return types.ListValueFrom(ctx, types.StringType, values)
}

func configureEmailInventoryClient(
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) *cpanelmail.Client {
	if request.ProviderData == nil {
		return nil
	}
	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return nil
	}
	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf(
				"Expected *email.Client, got: %T.",
				providerData["email"],
			),
		)
		return nil
	}

	return client
}
