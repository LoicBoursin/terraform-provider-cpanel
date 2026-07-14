package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type DNSRecordModel struct {
	Zone       types.String `tfsdk:"zone"`
	Name       types.String `tfsdk:"name"`
	RecordType types.String `tfsdk:"type"`
	TTL        types.Int64  `tfsdk:"ttl"`
	Data       types.List   `tfsdk:"data"`
	LineIndex  types.Int64  `tfsdk:"line_index"`
}
