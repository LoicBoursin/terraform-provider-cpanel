package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/passenger"
)

var (
	_ resource.Resource                = &passengerApplicationResource{}
	_ resource.ResourceWithConfigure   = &passengerApplicationResource{}
	_ resource.ResourceWithImportState = &passengerApplicationResource{}
)

func NewPassengerApplicationResource() resource.Resource {
	return &passengerApplicationResource{}
}

type passengerApplicationResource struct {
	client *passenger.Client
}

func (r *passengerApplicationResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_passenger_application"
}

func (r *passengerApplicationResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one cPanel Passenger application registration.",
		MarkdownDescription: "Manages one cPanel Passenger application registration. The application directory must already exist. Destroy unregisters Passenger without deleting the directory or installing dependencies.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The Passenger application name.",
				MarkdownDescription: "The Passenger application name.",
				Validators:          passengerApplicationNameValidators(),
			},
			"path": schema.StringAttribute{
				Required:            true,
				Description:         "The existing application directory relative to the cPanel account home.",
				MarkdownDescription: "The existing normalized application directory relative to the cPanel account home.",
				Validators:          passengerApplicationPathValidators(),
			},
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The existing account domain that serves the application.",
				MarkdownDescription: "The existing account domain that serves the application.",
				Validators:          domainNameValidators(),
			},
			"base_uri": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("/"),
				Description:         "The URL path that mounts the application on its domain.",
				MarkdownDescription: "The normalized URL path that mounts the application on its domain. Changing it replaces the registration because cPanel cannot edit it in place.",
				Validators:          passengerApplicationBaseURIValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"deployment_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default: stringdefault.StaticString(
					passenger.DeploymentModeProduction,
				),
				Description:         "The Passenger deployment mode.",
				MarkdownDescription: "The Passenger deployment mode: `production` or `development`.",
				Validators:          passengerDeploymentModeValidators(),
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				Description:         "Whether cPanel generates the web-server configuration for the application.",
				MarkdownDescription: "Whether cPanel generates the web-server configuration for the application.",
			},
			"environment_variables": schema.MapAttribute{
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				Description:         "The complete set of application environment variables.",
				MarkdownDescription: "The complete set of application environment variables. cPanel replaces the full set on update, and Terraform stores values as sensitive state.",
				Validators:          passengerEnvironmentVariableValidators(),
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"absolute_path": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute application path reported by cPanel.",
				MarkdownDescription: "The absolute application path reported by cPanel.",
			},
			"dependency_commands": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The dependency installation commands reported by cPanel.",
				MarkdownDescription: "The dependency installation commands reported by cPanel for `gem`, `npm`, and `pip`. The provider never executes them.",
			},
			"nodejs": schema.StringAttribute{
				Computed:            true,
				Description:         "The Node.js executable reported by cPanel, when present.",
				MarkdownDescription: "The Node.js executable reported by cPanel, when present.",
			},
			"python": schema.StringAttribute{
				Computed:            true,
				Description:         "The Python executable reported by cPanel, when present.",
				MarkdownDescription: "The Python executable reported by cPanel, when present.",
			},
			"ruby": schema.StringAttribute{
				Computed:            true,
				Description:         "The Ruby executable reported by cPanel, when present.",
				MarkdownDescription: "The Ruby executable reported by cPanel, when present.",
			},
		},
	}
}

func (r *passengerApplicationResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state PassengerApplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	application, err := r.client.Get(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Passenger application",
			err.Error(),
		)
		return
	}
	if application == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(
		applyPassengerApplicationToResourceModel(
			ctx,
			&state,
			*application,
		)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *passengerApplicationResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan PassengerApplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition, diagnostics := passengerApplicationDefinitionFromResourceModel(
		ctx,
		plan,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validatePassengerApplicationDefinition(definition); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Passenger application",
			err.Error(),
		)
		return
	}

	existing, err := r.client.Get(ctx, definition.Name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Passenger application",
			err.Error(),
		)
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Passenger application already exists",
			fmt.Sprintf(
				"Passenger application %q is already registered in cPanel. Import it instead of taking ownership implicitly.",
				definition.Name,
			),
		)
		return
	}
	pathExists, err := r.client.PathExists(ctx, definition.Path)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to inspect Passenger application path",
			err.Error(),
		)
		return
	}
	if !pathExists {
		resp.Diagnostics.AddError(
			"Passenger application path not found",
			fmt.Sprintf(
				"Directory %q must exist before Terraform registers the Passenger application.",
				definition.Path,
			),
		)
		return
	}

	createErr := r.client.Create(ctx, definition)
	if createErr != nil {
		if cPanelMutationErrorIsDeterministic(createErr) {
			resp.Diagnostics.AddError(
				"Unable to register Passenger application",
				passengerMutationErrorDetail(createErr, nil),
			)
			return
		}

		application, reconcileErr := r.verify(ctx, definition)
		if reconcileErr != nil {
			resp.Diagnostics.AddError(
				"Unable to reconcile Passenger application registration",
				fmt.Sprintf(
					"%s Reconciliation also failed: %v",
					createMutationErrorDetail(
						createErr,
						"Passenger application registration",
						fmt.Sprintf("Passenger application %q", definition.Name),
					),
					reconcileErr,
				),
			)
			return
		}

		resp.Diagnostics.Append(
			applyPassengerApplicationToResourceModel(
				ctx,
				&plan,
				*application,
			)...,
		)
		if resp.Diagnostics.HasError() {
			return
		}
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
		return
	}

	application, err := r.verify(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackCreated(ctx, definition)
		resp.Diagnostics.AddError(
			"Unable to verify Passenger application",
			passengerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyPassengerApplicationToResourceModel(
			ctx,
			&plan,
			*application,
		)...,
	)
	if resp.Diagnostics.HasError() {
		if rollbackErr := r.rollbackCreated(
			ctx,
			definition,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to unregister Passenger application",
				sensitiveMutationError(
					rollbackErr,
					"Passenger application rollback",
				).Error(),
			)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		if rollbackErr := r.rollbackCreated(
			ctx,
			definition,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to unregister Passenger application",
				sensitiveMutationError(
					rollbackErr,
					"Passenger application rollback",
				).Error(),
			)
		}
	}
}

func (r *passengerApplicationResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan PassengerApplicationResourceModel
	var state PassengerApplicationResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition, diagnostics := passengerApplicationDefinitionFromResourceModel(
		ctx,
		plan,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validatePassengerApplicationDefinition(definition); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Passenger application",
			err.Error(),
		)
		return
	}

	current, err := r.client.Get(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Passenger application",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Passenger application no longer exists",
			"Refresh the Terraform state before updating the Passenger application.",
		)
		return
	}
	previousDefinition, diagnostics :=
		passengerApplicationDefinitionFromResourceModel(ctx, state)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !current.Matches(previousDefinition) {
		resp.Diagnostics.AddError(
			"Passenger application changed outside Terraform",
			fmt.Sprintf(
				"Refusing to update Passenger application %q because its current cPanel definition no longer matches the Terraform state.",
				current.Name,
			),
		)
		return
	}

	if err := r.client.Update(
		ctx,
		current.Name,
		definition,
	); err != nil {
		rollbackErr := r.restore(
			ctx,
			*current,
			definition,
		)
		resp.Diagnostics.AddError(
			"Unable to update Passenger application",
			passengerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	application, err := r.verify(ctx, definition)
	if err != nil {
		rollbackErr := r.restore(
			ctx,
			*current,
			definition,
		)
		resp.Diagnostics.AddError(
			"Unable to verify Passenger application update",
			passengerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyPassengerApplicationToResourceModel(
			ctx,
			&plan,
			*application,
		)...,
	)
	if resp.Diagnostics.HasError() {
		if rollbackErr := r.restore(
			ctx,
			*current,
			definition,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to restore Passenger application",
				sensitiveMutationError(
					rollbackErr,
					"Passenger application rollback",
				).Error(),
			)
		}
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		if rollbackErr := r.restore(
			ctx,
			*current,
			definition,
		); rollbackErr != nil {
			resp.Diagnostics.AddError(
				"Unable to restore Passenger application",
				sensitiveMutationError(
					rollbackErr,
					"Passenger application rollback",
				).Error(),
			)
		}
	}
}

func (r *passengerApplicationResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state PassengerApplicationResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	expected, diagnostics := passengerApplicationDefinitionFromResourceModel(
		ctx,
		state,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	application, err := r.client.Get(ctx, expected.Name)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read Passenger application",
			err.Error(),
		)
		return
	}
	if application == nil {
		return
	}
	if !application.Matches(expected) {
		resp.Diagnostics.AddError(
			"Passenger application changed outside Terraform",
			fmt.Sprintf(
				"Refusing to unregister Passenger application %q because its current cPanel definition no longer matches the Terraform state.",
				expected.Name,
			),
		)
		return
	}
	deleteErr := r.client.Delete(ctx, expected.Name)
	application, readErr := r.client.Get(ctx, expected.Name)
	if readErr != nil {
		detail := "Could not verify the Passenger application removal: " +
			readErr.Error()
		if deleteErr != nil {
			detail = fmt.Sprintf(
				"Could not unregister Passenger application: %v. %s",
				deleteErr,
				detail,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify Passenger application removal",
			detail,
		)
		return
	}
	if application == nil {
		return
	}
	if !application.Matches(expected) {
		resp.Diagnostics.AddWarning(
			"Passenger application was replaced during deletion",
			fmt.Sprintf(
				"The managed Passenger application %q was unregistered, but another definition now uses the same name. Terraform left the replacement untouched.",
				expected.Name,
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to unregister Passenger application",
			deleteErr.Error(),
		)
		return
	}
	if application != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Passenger application removal",
			fmt.Sprintf(
				"Passenger application %q remains registered after cPanel reported success.",
				expected.Name,
			),
		)
	}
}

func (r *passengerApplicationResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := passenger.ValidateName(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Passenger application import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...,
	)
}

func (r *passengerApplicationResource) Configure(
	_ context.Context,
	req resource.ConfigureRequest,
	resp *resource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["passenger"].(*passenger.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Passenger Client Type",
			fmt.Sprintf(
				"Expected *passenger.Client, got: %T.",
				providerData["passenger"],
			),
		)
		return
	}

	r.client = client
}

func (r *passengerApplicationResource) verify(
	ctx context.Context,
	definition passenger.Definition,
) (*passenger.Application, error) {
	application, err := r.client.Get(ctx, definition.Name)
	if err != nil {
		return nil, fmt.Errorf(
			"read Passenger application after mutation: %w",
			err,
		)
	}
	if application == nil {
		return nil, fmt.Errorf(
			"passenger application %q is absent after cPanel reported success",
			definition.Name,
		)
	}
	if !application.Matches(definition) {
		return nil, fmt.Errorf(
			"passenger application %q does not match the requested configuration after cPanel reported success",
			definition.Name,
		)
	}

	return application, nil
}

func (r *passengerApplicationResource) rollbackCreated(
	ctx context.Context,
	expected passenger.Definition,
) error {
	application, err := r.client.Get(ctx, expected.Name)
	if err != nil {
		return fmt.Errorf(
			"read Passenger application before rollback: %w",
			err,
		)
	}
	if application == nil {
		return nil
	}
	if !application.Matches(expected) {
		return fmt.Errorf(
			"refuse to unregister Passenger application %q because its current configuration does not match the attempted registration",
			expected.Name,
		)
	}
	deleteErr := r.client.Delete(ctx, expected.Name)
	application, readErr := r.client.Get(ctx, expected.Name)
	if readErr != nil {
		if deleteErr != nil {
			return fmt.Errorf(
				"unregister partially-created Passenger application: %v; verify rollback: %w",
				deleteErr,
				readErr,
			)
		}
		return fmt.Errorf(
			"verify Passenger application rollback: %w",
			readErr,
		)
	}
	if application == nil {
		return nil
	}
	if !application.Matches(expected) {
		return fmt.Errorf(
			"passenger application %q was replaced during rollback; Terraform preserved the replacement",
			expected.Name,
		)
	}
	if deleteErr != nil {
		return fmt.Errorf(
			"unregister partially-created Passenger application: %w",
			deleteErr,
		)
	}
	if application != nil {
		return fmt.Errorf(
			"passenger application %q remains registered after rollback",
			expected.Name,
		)
	}

	return nil
}

func (r *passengerApplicationResource) restore(
	ctx context.Context,
	original passenger.Application,
	attempted passenger.Definition,
) error {
	originalDefinition := original.Definition()
	attemptedApplication, err := r.client.Get(ctx, attempted.Name)
	if err != nil {
		return fmt.Errorf(
			"read attempted Passenger application before restore: %w",
			err,
		)
	}

	if attempted.Name == original.Name {
		if attemptedApplication == nil {
			return fmt.Errorf(
				"passenger application %q is absent and cannot be restored",
				original.Name,
			)
		}
		if attemptedApplication.Matches(originalDefinition) {
			return nil
		}
		if !attemptedApplication.Matches(attempted) {
			return fmt.Errorf(
				"refuse to restore Passenger application %q because its current configuration matches neither the attempted nor the original definition",
				original.Name,
			)
		}

		return r.applyPassengerApplicationRestore(
			ctx,
			attemptedApplication.Name,
			attempted,
			originalDefinition,
		)
	}

	originalApplication, err := r.client.Get(ctx, original.Name)
	if err != nil {
		return fmt.Errorf(
			"read original Passenger application before restore: %w",
			err,
		)
	}
	if originalApplication != nil {
		if !originalApplication.Matches(originalDefinition) {
			return fmt.Errorf(
				"refuse to restore Passenger application %q because its original name is now used by a different configuration",
				original.Name,
			)
		}
		if attemptedApplication != nil {
			return fmt.Errorf(
				"refuse to restore Passenger application %q because both the original and attempted names are registered",
				original.Name,
			)
		}

		return nil
	}
	if attemptedApplication == nil {
		return fmt.Errorf(
			"passenger applications %q and %q are absent and cannot be restored",
			original.Name,
			attempted.Name,
		)
	}
	if !attemptedApplication.Matches(attempted) {
		return fmt.Errorf(
			"refuse to restore Passenger application %q because the attempted name is now used by a different configuration",
			original.Name,
		)
	}

	return r.applyPassengerApplicationRestore(
		ctx,
		attemptedApplication.Name,
		attempted,
		originalDefinition,
	)
}

func (r *passengerApplicationResource) applyPassengerApplicationRestore(
	ctx context.Context,
	currentName string,
	attempted passenger.Definition,
	original passenger.Definition,
) error {
	if err := r.client.Update(ctx, currentName, original); err != nil {
		restored, reconcileErr := r.passengerApplicationRestoreApplied(
			ctx,
			attempted,
			original,
		)
		if reconcileErr != nil {
			return fmt.Errorf(
				"restore previous Passenger application configuration: %w; reconcile restore: %v",
				err,
				reconcileErr,
			)
		}
		if restored {
			return nil
		}

		return fmt.Errorf(
			"restore previous Passenger application configuration: %w",
			err,
		)
	}
	if _, err := r.verify(ctx, original); err != nil {
		return fmt.Errorf(
			"verify restored Passenger application: %w",
			err,
		)
	}

	return nil
}

func (r *passengerApplicationResource) passengerApplicationRestoreApplied(
	ctx context.Context,
	attempted passenger.Definition,
	original passenger.Definition,
) (bool, error) {
	originalApplication, err := r.client.Get(ctx, original.Name)
	if err != nil {
		return false, fmt.Errorf(
			"read original Passenger application: %w",
			err,
		)
	}

	var attemptedApplication *passenger.Application
	if attempted.Name != original.Name {
		attemptedApplication, err = r.client.Get(ctx, attempted.Name)
		if err != nil {
			return false, fmt.Errorf(
				"read attempted Passenger application: %w",
				err,
			)
		}
	}

	if attempted.Name == original.Name {
		if originalApplication == nil {
			return false, nil
		}
		if originalApplication.Matches(original) {
			return true, nil
		}
		if originalApplication.Matches(attempted) {
			return false, nil
		}

		return false, fmt.Errorf(
			"name %q is used by a concurrent configuration",
			original.Name,
		)
	}
	if originalApplication != nil &&
		originalApplication.Matches(original) &&
		attemptedApplication == nil {
		return true, nil
	}
	if originalApplication != nil &&
		!originalApplication.Matches(original) {
		return false, fmt.Errorf(
			"original name %q is used by a concurrent configuration",
			original.Name,
		)
	}
	if attemptedApplication != nil &&
		!attemptedApplication.Matches(attempted) {
		return false, fmt.Errorf(
			"attempted name %q is used by a concurrent configuration",
			attempted.Name,
		)
	}
	if originalApplication != nil && attemptedApplication != nil {
		return false, fmt.Errorf(
			"both original name %q and attempted name %q are registered",
			original.Name,
			attempted.Name,
		)
	}

	return false, nil
}

func passengerMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	primaryError = sensitiveMutationError(
		primaryError,
		"Passenger application mutation",
	)
	rollbackError = sensitiveMutationError(
		rollbackError,
		"Passenger application rollback",
	)
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous Passenger application state: %v",
		primaryError,
		rollbackError,
	)
}
