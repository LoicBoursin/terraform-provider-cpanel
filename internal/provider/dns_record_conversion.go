package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldns "terraform-provider-cpanel/internal/cpanel/dns"
)

func dnsRecordData(ctx context.Context, value types.List) ([]string, diag.Diagnostics) {
	var data []string
	diagnostics := value.ElementsAs(ctx, &data, false)

	return data, diagnostics
}

func dnsRecordFromModel(
	ctx context.Context,
	model DNSRecordModel,
) (cpaneldns.Record, diag.Diagnostics) {
	data, diagnostics := dnsRecordData(ctx, model.Data)
	if diagnostics.HasError() {
		return cpaneldns.Record{}, diagnostics
	}

	return cpaneldns.Record{
		LineIndex: model.LineIndex.ValueInt64(),
		Name:      model.Name.ValueString(),
		Type:      model.RecordType.ValueString(),
		TTL:       model.TTL.ValueInt64(),
		Data:      data,
	}, diagnostics
}

func applyDNSRecordToModel(
	ctx context.Context,
	model *DNSRecordModel,
	record cpaneldns.Record,
) diag.Diagnostics {
	data, diagnostics := types.ListValueFrom(ctx, types.StringType, record.Data)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Name = types.StringValue(record.Name)
	model.RecordType = types.StringValue(record.Type)
	model.TTL = types.Int64Value(record.TTL)
	model.Data = data
	model.LineIndex = types.Int64Value(record.LineIndex)

	return diagnostics
}
