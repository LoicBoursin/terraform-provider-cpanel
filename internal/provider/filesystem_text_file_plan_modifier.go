package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type filesystemTextFileOwnershipMatchPlanModifier struct{}

func (filesystemTextFileOwnershipMatchPlanModifier) Description(
	_ context.Context,
) string {
	return "plans verified ownership for provider-owned files so a stale marker is reconciled"
}

func (m filesystemTextFileOwnershipMatchPlanModifier) MarkdownDescription(
	ctx context.Context,
) string {
	return m.Description(ctx)
}

func (filesystemTextFileOwnershipMatchPlanModifier) PlanModifyBool(
	ctx context.Context,
	request planmodifier.BoolRequest,
	response *planmodifier.BoolResponse,
) {
	if request.State.Raw.IsNull() ||
		request.Plan.Raw.IsNull() ||
		request.StateValue.IsNull() ||
		request.StateValue.IsUnknown() {
		return
	}

	var state FilesystemTextFileResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() ||
		state.Owned.IsNull() ||
		state.Owned.IsUnknown() {
		return
	}
	if state.Owned.ValueBool() {
		response.PlanValue = types.BoolValue(true)

		return
	}

	response.PlanValue = request.StateValue
}
