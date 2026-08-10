package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ddns"
)

type DynamicDNSResourceModel struct {
	Domain         types.String `tfsdk:"domain"`
	Description    types.String `tfsdk:"description"`
	WebcallID      types.String `tfsdk:"webcall_id"`
	WebcallURL     types.String `tfsdk:"webcall_url"`
	CreatedAt      types.Int64  `tfsdk:"created_at"`
	IPv4           types.Set    `tfsdk:"ipv4"`
	IPv6           types.Set    `tfsdk:"ipv6"`
	LastRunTimes   types.List   `tfsdk:"last_run_times"`
	LastUpdateTime types.Int64  `tfsdk:"last_update_time"`
}

type DynamicDNSDataSourceModel struct {
	Domain         types.String `tfsdk:"domain"`
	Description    types.String `tfsdk:"description"`
	WebcallID      types.String `tfsdk:"webcall_id"`
	WebcallURL     types.String `tfsdk:"webcall_url"`
	CreatedAt      types.Int64  `tfsdk:"created_at"`
	IPv4           types.Set    `tfsdk:"ipv4"`
	IPv6           types.Set    `tfsdk:"ipv6"`
	LastRunTimes   types.List   `tfsdk:"last_run_times"`
	LastUpdateTime types.Int64  `tfsdk:"last_update_time"`
}

func applyDynamicDNSToResourceModel(
	ctx context.Context,
	model *DynamicDNSResourceModel,
	domain ddns.Domain,
) diag.Diagnostics {
	ipv4, ipv6, lastRunTimes, diagnostics := dynamicDNSCollections(ctx, domain)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Domain = types.StringValue(domain.Domain)
	model.Description = types.StringValue(domain.Description)
	model.WebcallID = types.StringValue(domain.ID)
	model.WebcallURL = types.StringValue(dynamicDNSWebcallURL(domain))
	model.CreatedAt = types.Int64Value(domain.CreatedTime)
	model.IPv4 = ipv4
	model.IPv6 = ipv6
	model.LastRunTimes = lastRunTimes
	model.LastUpdateTime = dynamicDNSLastUpdateTime(domain)

	return diagnostics
}

func dynamicDNSToDataSourceModel(
	ctx context.Context,
	domain ddns.Domain,
) (*DynamicDNSDataSourceModel, diag.Diagnostics) {
	ipv4, ipv6, lastRunTimes, diagnostics := dynamicDNSCollections(ctx, domain)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &DynamicDNSDataSourceModel{
		Domain:         types.StringValue(domain.Domain),
		Description:    types.StringValue(domain.Description),
		WebcallID:      types.StringValue(domain.ID),
		WebcallURL:     types.StringValue(dynamicDNSWebcallURL(domain)),
		CreatedAt:      types.Int64Value(domain.CreatedTime),
		IPv4:           ipv4,
		IPv6:           ipv6,
		LastRunTimes:   lastRunTimes,
		LastUpdateTime: dynamicDNSLastUpdateTime(domain),
	}, diagnostics
}

func dynamicDNSCollections(
	ctx context.Context,
	domain ddns.Domain,
) (types.Set, types.Set, types.List, diag.Diagnostics) {
	ipv4 := domain.IPv4
	if ipv4 == nil {
		ipv4 = []string{}
	}
	ipv4Set, diagnostics := types.SetValueFrom(ctx, types.StringType, ipv4)
	if diagnostics.HasError() {
		return types.Set{}, types.Set{}, types.List{}, diagnostics
	}

	ipv6 := domain.IPv6
	if ipv6 == nil {
		ipv6 = []string{}
	}
	ipv6Set, ipv6Diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		ipv6,
	)
	diagnostics.Append(ipv6Diagnostics...)
	if diagnostics.HasError() {
		return types.Set{}, types.Set{}, types.List{}, diagnostics
	}

	lastRunTimes := domain.LastRunTimes
	if lastRunTimes == nil {
		lastRunTimes = []int64{}
	}
	lastRunTimesList, lastRunDiagnostics := types.ListValueFrom(
		ctx,
		types.Int64Type,
		lastRunTimes,
	)
	diagnostics.Append(lastRunDiagnostics...)

	return ipv4Set, ipv6Set, lastRunTimesList, diagnostics
}

func dynamicDNSLastUpdateTime(domain ddns.Domain) types.Int64 {
	if domain.LastUpdateTime == nil {
		return types.Int64Null()
	}

	return types.Int64Value(*domain.LastUpdateTime)
}

func dynamicDNSWebcallURL(domain ddns.Domain) string {
	return "https://" + domain.Domain + "/cpanelwebcall/" + domain.ID
}
