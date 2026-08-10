package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

type singletonUpdateFunc func(
	context.Context,
	frameworkresource.UpdateRequest,
	*frameworkresource.UpdateResponse,
)

func runSingletonUpdate[StateModel, PlanModel any](
	t *testing.T,
	schemaResource frameworkresource.Resource,
	stateModel StateModel,
	planModel PlanModel,
	update singletonUpdateFunc,
) *frameworkresource.UpdateResponse {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	schemaResource.Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResponse.Schema}
	if diagnostics := state.Set(
		t.Context(),
		&stateModel,
	); diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}
	plan := tfsdk.Plan{Schema: schemaResponse.Schema}
	if diagnostics := plan.Set(
		t.Context(),
		&planModel,
	); diagnostics.HasError() {
		t.Fatalf("Plan.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.UpdateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	update(
		t.Context(),
		frameworkresource.UpdateRequest{
			State: state,
			Plan:  plan,
		},
		response,
	)

	return response
}

func assertUpdateDriftRefused(
	t *testing.T,
	diagnostics diag.Diagnostics,
) {
	t.Helper()

	for _, diagnostic := range diagnostics {
		if strings.Contains(
			diagnostic.Summary(),
			"changed during update",
		) {
			return
		}
	}

	t.Fatalf("Update() diagnostics = %v, want a drift refusal", diagnostics)
}
