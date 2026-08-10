package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/apachehandler"
)

var (
	_ resource.Resource                = &apacheHandlerResource{}
	_ resource.ResourceWithConfigure   = &apacheHandlerResource{}
	_ resource.ResourceWithImportState = &apacheHandlerResource{}
)

func NewApacheHandlerResource() resource.Resource {
	return &apacheHandlerResource{}
}

type apacheHandlerResource struct {
	client *apachehandler.Client
}

func (r *apacheHandlerResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_apache_handler"
}

func (r *apacheHandlerResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one custom cPanel Apache handler for a file extension.",
		MarkdownDescription: "Manages one custom cPanel Apache handler for a file extension.",
		Attributes: map[string]schema.Attribute{
			"extension": schema.StringAttribute{
				Required:            true,
				Description:         "The file extension handled by Apache.",
				MarkdownDescription: "The file extension handled by Apache. It must begin with a dot.",
				Validators:          mimeExtensionValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"handler": schema.StringAttribute{
				Required:            true,
				Description:         "The Apache handler name.",
				MarkdownDescription: "The Apache handler name.",
				Validators:          apacheHandlerValidators(),
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				Description:         "The Apache handler owner reported by cPanel.",
				MarkdownDescription: "The Apache handler owner reported by cPanel. Managed entries report `user`.",
			},
		},
	}
}

func (r *apacheHandlerResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state ApacheHandlerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiHandler, err := r.client.Get(ctx, state.Extension.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Apache handler", err.Error())
		return
	}
	if apiHandler == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyApacheHandlerToResourceModel(&state, *apiHandler)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apacheHandlerResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan ApacheHandlerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := apacheHandlerDefinitionFromResourceModel(plan)
	if err := validateApacheHandlerDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid Apache handler", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, definition.Extension)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Apache handler", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Apache handler already exists",
			fmt.Sprintf(
				"An Apache handler for extension %q already exists. Import it instead of replacing it implicitly.",
				definition.Extension,
			),
		)
		return
	}

	if err := r.client.Add(ctx, definition); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Apache handler",
			"Could not create Apache handler: "+err.Error(),
		)
		return
	}

	apiHandler, err := r.verifyApacheHandler(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackCreatedApacheHandler(ctx, definition)
		resp.Diagnostics.AddError(
			"Unable to verify Apache handler",
			apacheHandlerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyApacheHandlerToResourceModel(&plan, *apiHandler)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apacheHandlerResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan ApacheHandlerResourceModel
	var state ApacheHandlerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.Get(ctx, state.Extension.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Apache handler", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Apache handler no longer exists",
			"Refresh the Terraform state before updating the Apache handler.",
		)
		return
	}

	oldDefinition := apacheHandlerDefinitionFromResourceModel(state)
	if !apacheHandlerMatchesDefinition(current, oldDefinition) {
		resp.Diagnostics.AddError(
			"Apache handler changed outside Terraform",
			"The remote Apache handler no longer matches Terraform state. Refresh and review the drift before retrying.",
		)
		return
	}
	newDefinition := apacheHandlerDefinitionFromResourceModel(plan)
	if err := validateApacheHandlerDefinition(newDefinition); err != nil {
		resp.Diagnostics.AddError("Invalid Apache handler", err.Error())
		return
	}

	if err := r.deleteApacheHandlerDefinition(
		ctx,
		oldDefinition,
		nil,
		"delete the previous Apache handler",
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to replace Apache handler",
			"Could not delete the previous Apache handler: "+err.Error(),
		)
		return
	}
	if err := r.client.Add(ctx, newDefinition); err != nil {
		rollbackErr := r.restoreApacheHandler(
			ctx,
			newDefinition,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to replace Apache handler",
			apacheHandlerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	apiHandler, err := r.verifyApacheHandler(ctx, newDefinition)
	if err != nil {
		rollbackErr := r.restoreApacheHandler(
			ctx,
			newDefinition,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to verify Apache handler replacement",
			apacheHandlerMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyApacheHandlerToResourceModel(&plan, *apiHandler)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apacheHandlerResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state ApacheHandlerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	extension := state.Extension.ValueString()
	existing, err := r.client.Get(ctx, extension)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Apache handler", err.Error())
		return
	}
	if existing == nil {
		return
	}
	expected := apacheHandlerDefinitionFromResourceModel(state)
	if existing.Origin != "user" ||
		apacheHandlerDefinitionFromAPI(*existing) != expected {
		resp.Diagnostics.AddError(
			"Unable to delete Apache handler",
			"The remote Apache handler no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteApacheHandlerDefinition(
		ctx,
		expected,
		nil,
		"delete the Apache handler",
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Apache handler",
			err.Error(),
		)
	}
}

func (r *apacheHandlerResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if !mimeExtensionPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError(
			"Invalid Apache handler import identifier",
			"Expected a file extension beginning with a dot.",
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("extension"), req.ID)...,
	)
}

func (r *apacheHandlerResource) Configure(
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
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["apachehandler"].(*apachehandler.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Apache Handler Client Type",
			fmt.Sprintf(
				"Expected *apachehandler.Client, got: %T.",
				providerData["apachehandler"],
			),
		)
		return
	}

	r.client = client
}

func (r *apacheHandlerResource) verifyApacheHandler(
	ctx context.Context,
	expected apachehandler.Definition,
) (*apachehandler.Handler, error) {
	apiHandler, err := r.client.Get(ctx, expected.Extension)
	if err != nil {
		return nil, fmt.Errorf("read Apache handler after mutation: %w", err)
	}
	if apiHandler == nil {
		return nil, fmt.Errorf(
			"apache handler for extension %q was not found after mutation",
			expected.Extension,
		)
	}

	actual := apacheHandlerDefinitionFromAPI(*apiHandler)
	if actual != expected {
		return nil, fmt.Errorf(
			"apache handler for extension %q is %q; expected %q",
			expected.Extension,
			actual.Handler,
			expected.Handler,
		)
	}
	if apiHandler.Origin != "user" {
		return nil, fmt.Errorf(
			"apache handler for extension %q origin is %q; expected user",
			expected.Extension,
			apiHandler.Origin,
		)
	}

	return apiHandler, nil
}

func (r *apacheHandlerResource) rollbackCreatedApacheHandler(
	ctx context.Context,
	attempted apachehandler.Definition,
) error {
	return r.deleteApacheHandlerDefinition(
		ctx,
		attempted,
		nil,
		"delete the created Apache handler",
	)
}

func (r *apacheHandlerResource) restoreApacheHandler(
	ctx context.Context,
	attempted apachehandler.Definition,
	original apachehandler.Definition,
) error {
	current, err := r.client.Get(ctx, attempted.Extension)
	if err != nil {
		return fmt.Errorf("read replacement Apache handler before rollback: %w", err)
	}
	if current != nil {
		if apacheHandlerMatchesDefinition(current, original) {
			_, err := r.verifyApacheHandler(ctx, original)
			if err != nil {
				return fmt.Errorf("verify restored Apache handler: %w", err)
			}

			return nil
		}
		if !apacheHandlerMatchesDefinition(current, attempted) {
			return fmt.Errorf(
				"refuse to restore the previous Apache handler because the current replacement no longer matches the attempted definition",
			)
		}
		if err := r.deleteApacheHandlerDefinition(
			ctx,
			attempted,
			&original,
			"delete the replacement Apache handler",
		); err != nil {
			return err
		}

		current, err = r.client.Get(ctx, original.Extension)
		if err != nil {
			return fmt.Errorf(
				"read Apache handler after deleting replacement: %w",
				err,
			)
		}
		if current != nil {
			if !apacheHandlerMatchesDefinition(current, original) {
				return fmt.Errorf(
					"refuse to restore the previous Apache handler because a concurrent definition appeared after deleting the replacement",
				)
			}
			if _, err := r.verifyApacheHandler(ctx, original); err != nil {
				return fmt.Errorf("verify restored Apache handler: %w", err)
			}

			return nil
		}
	}
	if err := r.client.Add(ctx, original); err != nil {
		current, readErr := r.client.Get(ctx, original.Extension)
		if readErr != nil {
			return fmt.Errorf(
				"restore previous Apache handler: %v; read Apache handler after ambiguous restore: %w",
				err,
				readErr,
			)
		}
		if apacheHandlerMatchesDefinition(current, original) {
			return nil
		}
		if current != nil {
			return fmt.Errorf(
				"restore previous Apache handler: %v; refuse to overwrite a concurrent Apache handler definition",
				err,
			)
		}

		return fmt.Errorf("restore previous Apache handler: %w", err)
	}
	if _, err := r.verifyApacheHandler(ctx, original); err != nil {
		return fmt.Errorf("verify restored Apache handler: %w", err)
	}

	return nil
}

func (r *apacheHandlerResource) deleteApacheHandlerDefinition(
	ctx context.Context,
	expected apachehandler.Definition,
	alreadyRestored *apachehandler.Definition,
	operation string,
) error {
	current, err := r.client.Get(ctx, expected.Extension)
	if err != nil {
		return fmt.Errorf("read Apache handler before %s: %w", operation, err)
	}
	if current == nil {
		return nil
	}
	if alreadyRestored != nil &&
		apacheHandlerMatchesDefinition(current, *alreadyRestored) {
		return nil
	}
	if !apacheHandlerMatchesDefinition(current, expected) {
		return fmt.Errorf(
			"refuse to %s because the current Apache handler no longer matches the expected definition",
			operation,
		)
	}

	deleteErr := r.client.Delete(ctx, expected.Extension)
	remaining, readErr := r.client.Get(ctx, expected.Extension)
	if readErr != nil {
		if deleteErr != nil {
			return fmt.Errorf(
				"%s: %v; reconcile the ambiguous deletion: %w",
				operation,
				deleteErr,
				readErr,
			)
		}

		return fmt.Errorf("verify %s: %w", operation, readErr)
	}
	if remaining == nil {
		return nil
	}
	if alreadyRestored != nil &&
		apacheHandlerMatchesDefinition(remaining, *alreadyRestored) {
		return nil
	}
	if apacheHandlerMatchesDefinition(remaining, expected) {
		if deleteErr != nil {
			return fmt.Errorf("%s: %w", operation, deleteErr)
		}

		return fmt.Errorf(
			"%s reported success but the expected Apache handler still exists",
			operation,
		)
	}

	return fmt.Errorf(
		"refuse to continue after %s because a concurrent Apache handler definition is now present",
		operation,
	)
}

func apacheHandlerMatchesDefinition(
	handler *apachehandler.Handler,
	expected apachehandler.Definition,
) bool {
	return handler != nil &&
		handler.Origin == "user" &&
		apacheHandlerDefinitionFromAPI(*handler) == expected
}

func apacheHandlerMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the Apache handler change: %v",
		primaryError,
		rollbackError,
	)
}
