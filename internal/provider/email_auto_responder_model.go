package provider

import "github.com/hashicorp/terraform-plugin-framework/types"

type EmailAutoResponderModel struct {
	Email         types.String `tfsdk:"email"`
	From          types.String `tfsdk:"from"`
	Subject       types.String `tfsdk:"subject"`
	Body          types.String `tfsdk:"body"`
	Charset       types.String `tfsdk:"charset"`
	IntervalHours types.Int64  `tfsdk:"interval_hours"`
	IsHTML        types.Bool   `tfsdk:"is_html"`
	StartUnix     types.Int64  `tfsdk:"start_unix"`
	StopUnix      types.Int64  `tfsdk:"stop_unix"`
}
