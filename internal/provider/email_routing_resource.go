package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailRoutingResource{}
	_ resource.ResourceWithConfigure   = &emailRoutingResource{}
	_ resource.ResourceWithImportState = &emailRoutingResource{}
)

func NewEmailRoutingResource() resource.Resource {
	return &emailRoutingResource{}
}

type emailRoutingClient interface {
	GetRouting(context.Context, string) (*cpanelmail.Routing, error)
	SetRouting(
		context.Context,
		cpanelmail.RoutingDefinition,
	) (*cpanelmail.Routing, []string, error)
	LockRoutingDomain(string) func()
}

type emailRoutingResource struct {
	client emailRoutingClient
}

func (r *emailRoutingResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_routing"
}

func (r *emailRoutingResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages cPanel email routing for an existing account domain.",
		MarkdownDescription: "Manages cPanel email routing for an existing account domain. This changes cPanel's local delivery configuration, does not edit MX DNS records, and restores the mode observed when Terraform first took ownership.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The existing account domain to manage.",
				MarkdownDescription: "The existing account domain to manage.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"mode": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel email routing mode.",
				MarkdownDescription: "The cPanel email routing mode: `auto`, `local`, `backup`, or `remote`.",
				Validators:          emailRoutingModeValidators(),
			},
			"detected_mode": schema.StringAttribute{
				Computed:            true,
				Description:         "The effective routing mode detected from the highest-priority mail exchanger.",
				MarkdownDescription: "The effective routing mode detected from the highest-priority mail exchanger.",
			},
			"primary_exchanger": schema.StringAttribute{
				Computed:            true,
				Description:         "The highest-priority mail exchanger reported by cPanel.",
				MarkdownDescription: "The highest-priority mail exchanger reported by cPanel.",
			},
			"restore_mode": schema.StringAttribute{
				Computed:            true,
				Description:         "The email routing mode that Terraform restores when the resource is removed.",
				MarkdownDescription: "The email routing mode that Terraform restores when the resource is removed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *emailRoutingResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state EmailRoutingResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	domain := state.Domain.ValueString()
	unlock := r.client.LockRoutingDomain(domain)
	defer unlock()

	current, err := r.client.GetRouting(ctx, domain)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email routing",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.State.RemoveResource(ctx)
		return
	}

	if emailRoutingRestoreModeMissing(state) {
		state.RestoreMode = types.StringValue(string(current.Mode))
	}
	applyEmailRoutingToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *emailRoutingResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan EmailRoutingResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	definition := emailRoutingDefinitionFromResourceModel(plan)
	if err := validateEmailRoutingDefinition(definition); err != nil {
		response.Diagnostics.AddError("Invalid cPanel email routing", err.Error())
		return
	}

	unlock := r.client.LockRoutingDomain(definition.Domain)
	defer unlock()

	original, err := r.client.GetRouting(ctx, definition.Domain)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email routing",
			err.Error(),
		)
		return
	}
	if original == nil {
		response.Diagnostics.AddError(
			"cPanel email routing not found",
			fmt.Sprintf(
				"Domain %q must exist in the cPanel mail routing inventory before Terraform can manage it.",
				definition.Domain,
			),
		)
		return
	}

	updated, warnings, err := r.transition(
		ctx,
		*original,
		definition.Mode,
	)
	addEmailRoutingWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to configure cPanel email routing",
			err.Error(),
		)
		return
	}

	plan.RestoreMode = types.StringValue(string(original.Mode))
	applyEmailRoutingToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() && original.Mode != definition.Mode {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			definition.Domain,
			definition.Mode,
			original.Mode,
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel email routing",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *emailRoutingResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan EmailRoutingResourceModel
	var state EmailRoutingResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	definition := emailRoutingDefinitionFromResourceModel(plan)
	if err := validateEmailRoutingDefinition(definition); err != nil {
		response.Diagnostics.AddError("Invalid cPanel email routing", err.Error())
		return
	}

	unlock := r.client.LockRoutingDomain(definition.Domain)
	defer unlock()

	original, err := r.client.GetRouting(ctx, definition.Domain)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel email routing",
			err.Error(),
		)
		return
	}
	if original == nil {
		response.Diagnostics.AddError(
			"cPanel email routing no longer exists",
			"Refresh the Terraform state before updating the email routing mode.",
		)
		return
	}
	if original.Mode != cpanelmail.RoutingMode(state.Mode.ValueString()) {
		response.Diagnostics.AddError(
			"cPanel email routing changed during update",
			"The current cPanel email routing mode no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}

	updated, warnings, err := r.transition(
		ctx,
		*original,
		definition.Mode,
	)
	addEmailRoutingWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to update cPanel email routing",
			err.Error(),
		)
		return
	}

	if emailRoutingRestoreModeMissing(state) {
		plan.RestoreMode = types.StringValue(string(original.Mode))
	} else {
		plan.RestoreMode = state.RestoreMode
	}
	applyEmailRoutingToResourceModel(&plan, *updated)
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() && original.Mode != definition.Mode {
		if rollbackErr := r.restoreIfCurrentMatches(
			ctx,
			definition.Domain,
			definition.Mode,
			original.Mode,
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore cPanel email routing",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *emailRoutingResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state EmailRoutingResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}
	if emailRoutingRestoreModeMissing(state) {
		response.Diagnostics.AddError(
			"Unable to restore cPanel email routing",
			"The resource state does not contain the email routing mode that preceded Terraform management.",
		)
		return
	}

	domain := state.Domain.ValueString()
	unlock := r.client.LockRoutingDomain(domain)
	defer unlock()

	err := r.restoreIfCurrentMatches(
		ctx,
		domain,
		cpanelmail.RoutingMode(state.Mode.ValueString()),
		cpanelmail.RoutingMode(state.RestoreMode.ValueString()),
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to restore cPanel email routing",
			err.Error(),
		)
	}
}

func (r *emailRoutingResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	definition := cpanelmail.RoutingDefinition{
		Domain: request.ID,
		Mode:   cpanelmail.RoutingModeAuto,
	}
	if err := validateEmailRoutingDefinition(definition); err != nil {
		response.Diagnostics.AddError(
			"Invalid cPanel email routing import identifier",
			err.Error(),
		)
		return
	}

	unlock := r.client.LockRoutingDomain(definition.Domain)
	defer unlock()

	current, err := r.client.GetRouting(ctx, definition.Domain)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import cPanel email routing",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"cPanel email routing not found",
			fmt.Sprintf(
				"Domain %q is not present in the cPanel mail routing inventory.",
				definition.Domain,
			),
		)
		return
	}

	state := EmailRoutingResourceModel{
		RestoreMode: types.StringValue(string(current.Mode)),
	}
	applyEmailRoutingToResourceModel(&state, *current)
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *emailRoutingResource) Configure(
	_ context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
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
		return
	}

	r.client = client
}

func (r *emailRoutingResource) transition(
	ctx context.Context,
	original cpanelmail.Routing,
	target cpanelmail.RoutingMode,
) (*cpanelmail.Routing, []string, error) {
	definition := cpanelmail.RoutingDefinition{
		Domain: original.Domain,
		Mode:   target,
	}
	if err := validateEmailRoutingDefinition(definition); err != nil {
		return nil, nil, err
	}
	if cpanelmail.RoutingMatchesDefinition(original, definition) {
		return &original, nil, nil
	}

	updated, warnings, err := r.client.SetRouting(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackFailedTransition(
			ctx,
			original,
			definition,
			err,
		)
		return nil, warnings, errors.New(
			emailRoutingMutationErrorDetail(err, rollbackErr),
		)
	}

	return updated, warnings, nil
}

func (r *emailRoutingResource) rollbackFailedTransition(
	ctx context.Context,
	original cpanelmail.Routing,
	target cpanelmail.RoutingDefinition,
	mutationErr error,
) error {
	current, err := r.client.GetRouting(ctx, original.Domain)
	if err != nil {
		return fmt.Errorf(
			"read cPanel email routing after failed mutation: %w",
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"refuse to restore the previous cPanel email routing because domain %q disappeared after the failed mutation",
			original.Domain,
		)
	}
	if current.Mode == original.Mode {
		return nil
	}
	if emailRoutingMutationErrorIsDeterministic(mutationErr) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel email routing because cPanel rejected the mutation but the current mode changed independently",
		)
	}
	if !cpanelmail.RoutingMatchesDefinition(*current, target) {
		return fmt.Errorf(
			"refuse to restore the previous cPanel email routing because the current mode matches neither the requested transition nor the previous mode",
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		target.Mode,
		original.Mode,
	)
}

func (r *emailRoutingResource) restoreIfCurrentMatches(
	ctx context.Context,
	domain string,
	expectedCurrent cpanelmail.RoutingMode,
	target cpanelmail.RoutingMode,
) error {
	if err := validateEmailRoutingDefinition(
		cpanelmail.RoutingDefinition{
			Domain: domain,
			Mode:   expectedCurrent,
		},
	); err != nil {
		return err
	}
	if err := cpanelmail.ValidateRoutingDefinition(
		cpanelmail.RoutingDefinition{
			Domain: domain,
			Mode:   target,
		},
	); err != nil {
		return err
	}

	current, err := r.client.GetRouting(ctx, domain)
	if err != nil {
		return fmt.Errorf(
			"read cPanel email routing before guarded restore: %w",
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"refuse to restore cPanel email routing because domain %q is no longer present in the mail routing inventory",
			domain,
		)
	}

	return r.restoreObservedTransition(
		ctx,
		*current,
		expectedCurrent,
		target,
	)
}

func (r *emailRoutingResource) restoreObservedTransition(
	ctx context.Context,
	current cpanelmail.Routing,
	expectedCurrent cpanelmail.RoutingMode,
	target cpanelmail.RoutingMode,
) error {
	if current.Mode == target {
		return nil
	}
	if current.Mode != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore cPanel email routing because the current mode no longer matches the Terraform transition",
		)
	}

	_, _, err := r.client.SetRouting(
		ctx,
		cpanelmail.RoutingDefinition{
			Domain: current.Domain,
			Mode:   target,
		},
	)
	if err != nil {
		return fmt.Errorf("restore previous cPanel email routing: %w", err)
	}

	return nil
}

func (r *emailRoutingResource) restore(
	ctx context.Context,
	domain string,
	target cpanelmail.RoutingMode,
) ([]string, error) {
	definition := cpanelmail.RoutingDefinition{
		Domain: domain,
		Mode:   target,
	}
	if err := validateEmailRoutingDefinition(definition); err != nil {
		return nil, err
	}

	current, err := r.client.GetRouting(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf(
			"read cPanel email routing before restore: %w",
			err,
		)
	}
	if current == nil || current.Mode == target {
		return nil, nil
	}
	_, warnings, err := r.client.SetRouting(ctx, definition)
	if err != nil {
		return warnings, fmt.Errorf(
			"restore previous cPanel email routing: %w",
			err,
		)
	}

	return warnings, nil
}

func addEmailRoutingWarnings(
	diagnostics *diag.Diagnostics,
	warnings []string,
) {
	for _, warning := range warnings {
		diagnostics.AddWarning(
			"cPanel email routing warning",
			warning,
		)
	}
}

func emailRoutingMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func emailRoutingMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel email routing: %v",
		primaryError,
		rollbackError,
	)
}
