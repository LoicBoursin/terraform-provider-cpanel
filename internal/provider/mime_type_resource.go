package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/mimetype"
)

var (
	_ resource.Resource                = &mimeTypeResource{}
	_ resource.ResourceWithConfigure   = &mimeTypeResource{}
	_ resource.ResourceWithImportState = &mimeTypeResource{}
)

func NewMIMETypeResource() resource.Resource {
	return &mimeTypeResource{}
}

type mimeTypeResource struct {
	client *mimetype.Client
}

type mimeTypeApplyTrace struct {
	definition           mimetype.Definition
	attributablePrefixes []mimetype.Definition
}

func (r *mimeTypeResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mime_type"
}

func (r *mimeTypeResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one custom cPanel Apache MIME type and its file extensions.",
		MarkdownDescription: "Manages one custom cPanel Apache MIME type and its file extensions.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required:            true,
				Description:         "The lowercase custom media type.",
				MarkdownDescription: "The lowercase custom media type, for example `application/x-example`.",
				Validators:          mimeTypeValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"extensions": schema.SetAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The file extensions associated with the MIME type.",
				MarkdownDescription: "The file extensions associated with the MIME type. Each extension must begin with a dot.",
				Validators: []validator.Set{
					setvalidator.SizeAtLeast(1),
					setvalidator.NoNullValues(),
					setvalidator.ValueStringsAre(
						mimeExtensionValidators()...,
					),
				},
			},
			"origin": schema.StringAttribute{
				Computed:            true,
				Description:         "The MIME type owner reported by cPanel.",
				MarkdownDescription: "The MIME type owner reported by cPanel. Managed entries report `user`.",
			},
		},
	}
}

func (r *mimeTypeResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state MIMETypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiMIMEType, err := r.client.Get(ctx, state.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MIME type", err.Error())
		return
	}
	if apiMIMEType == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(
		applyMIMETypeToResourceModel(ctx, &state, *apiMIMEType)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mimeTypeResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan MIMETypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition, diagnostics := mimeTypeDefinitionFromResourceModel(ctx, plan)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateMIMETypeDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid MIME type", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, definition.Type)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MIME type", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"MIME type already exists",
			fmt.Sprintf(
				"MIME type %q already exists. Import it instead of replacing it implicitly.",
				definition.Type,
			),
		)
		return
	}

	application, err := r.applyDefinition(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackCreatedMIMEType(ctx, application)
		resp.Diagnostics.AddError(
			"Unable to create MIME type",
			mimeTypeMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	apiMIMEType, err := r.verifyMIMEType(ctx, definition)
	if err != nil {
		rollbackErr := r.rollbackCreatedMIMEType(ctx, application)
		resp.Diagnostics.AddError(
			"Unable to verify MIME type",
			mimeTypeMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyMIMETypeToResourceModel(ctx, &plan, *apiMIMEType)...,
	)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreatedMIMEType(ctx, application)
		resp.Diagnostics.AddError(
			"Unable to decode MIME type",
			mimeTypeMutationErrorDetail(
				fmt.Errorf("convert MIME type metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mimeTypeResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan MIMETypeResourceModel
	var state MIMETypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	current, err := r.client.Get(ctx, state.Type.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MIME type", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"MIME type no longer exists",
			"Refresh the Terraform state before updating the MIME type.",
		)
		return
	}

	oldDefinition, diagnostics := mimeTypeDefinitionFromResourceModel(ctx, state)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !mimeTypeMatchesDefinition(current, oldDefinition) {
		resp.Diagnostics.AddError(
			"MIME type changed outside Terraform",
			"The remote MIME type no longer matches Terraform state. Refresh and review the drift before retrying.",
		)
		return
	}
	newDefinition, diagnostics := mimeTypeDefinitionFromResourceModel(ctx, plan)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := validateMIMETypeDefinition(newDefinition); err != nil {
		resp.Diagnostics.AddError("Invalid MIME type", err.Error())
		return
	}

	if err := r.deleteMIMETypeDefinition(
		ctx,
		oldDefinition,
		nil,
		"delete the previous MIME type",
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to replace MIME type",
			"Could not delete the previous MIME type: "+err.Error(),
		)
		return
	}
	application, err := r.applyDefinition(ctx, newDefinition)
	if err != nil {
		rollbackErr := r.restoreMIMEType(
			ctx,
			application,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to replace MIME type",
			mimeTypeMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	apiMIMEType, err := r.verifyMIMEType(ctx, newDefinition)
	if err != nil {
		rollbackErr := r.restoreMIMEType(
			ctx,
			application,
			oldDefinition,
		)
		resp.Diagnostics.AddError(
			"Unable to verify MIME type replacement",
			mimeTypeMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(
		applyMIMETypeToResourceModel(ctx, &plan, *apiMIMEType)...,
	)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mimeTypeResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state MIMETypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	mimeTypeName := state.Type.ValueString()
	existing, err := r.client.Get(ctx, mimeTypeName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read MIME type", err.Error())
		return
	}
	if existing == nil {
		return
	}
	expected, diagnostics := mimeTypeDefinitionFromResourceModel(ctx, state)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	if existing.Origin != "user" ||
		!mimeTypeDefinitionsEqual(
			mimeTypeDefinitionFromAPI(*existing),
			expected,
		) {
		resp.Diagnostics.AddError(
			"Unable to delete MIME type",
			"The remote MIME type no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteMIMETypeDefinition(
		ctx,
		expected,
		nil,
		"delete the MIME type",
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete MIME type",
			err.Error(),
		)
	}
}

func (r *mimeTypeResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if !mimeTypePattern.MatchString(req.ID) {
		resp.Diagnostics.AddError(
			"Invalid MIME type import identifier",
			"Expected a lowercase media type such as application/x-example.",
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("type"), req.ID)...,
	)
}

func (r *mimeTypeResource) Configure(
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

	client, ok := providerData["mimetype"].(*mimetype.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected MIME Type Client Type",
			fmt.Sprintf(
				"Expected *mimetype.Client, got: %T.",
				providerData["mimetype"],
			),
		)
		return
	}

	r.client = client
}

func (r *mimeTypeResource) applyDefinition(
	ctx context.Context,
	definition mimetype.Definition,
) (mimeTypeApplyTrace, error) {
	definition = definition.Sorted()
	trace := mimeTypeApplyTrace{definition: definition}

	for index, extension := range definition.Extensions {
		current, err := r.client.Get(ctx, definition.Type)
		if err != nil {
			return trace, fmt.Errorf(
				"read MIME type %q before adding extension %q: %w",
				definition.Type,
				extension,
				err,
			)
		}
		if index == 0 {
			if current != nil {
				return trace, fmt.Errorf(
					"refuse to add extension %q to MIME type %q because a concurrent definition is now present",
					extension,
					definition.Type,
				)
			}
		} else {
			previousPrefix := mimetype.Definition{
				Type: definition.Type,
				Extensions: append(
					[]string(nil),
					definition.Extensions[:index]...,
				),
			}
			if !mimeTypeMatchesDefinition(current, previousPrefix) {
				return trace, fmt.Errorf(
					"refuse to add extension %q to MIME type %q because the current definition no longer matches the attributable prefix",
					extension,
					definition.Type,
				)
			}
		}

		prefix := mimetype.Definition{
			Type: definition.Type,
			Extensions: append(
				[]string(nil),
				definition.Extensions[:index+1]...,
			),
		}
		err = r.client.AddExtension(
			ctx,
			definition.Type,
			extension,
		)
		if err != nil {
			return trace, fmt.Errorf(
				"add extension %q to MIME type %q: %w",
				extension,
				definition.Type,
				err,
			)
		}
		trace.addAttributablePrefix(prefix)
	}

	return trace, nil
}

func (r *mimeTypeResource) verifyMIMEType(
	ctx context.Context,
	expected mimetype.Definition,
) (*mimetype.MIMEType, error) {
	apiMIMEType, err := r.client.Get(ctx, expected.Type)
	if err != nil {
		return nil, fmt.Errorf("read MIME type after mutation: %w", err)
	}
	if apiMIMEType == nil {
		return nil, fmt.Errorf(
			"MIME type %q was not found after mutation",
			expected.Type,
		)
	}

	actual := mimeTypeDefinitionFromAPI(*apiMIMEType)
	expected = expected.Sorted()
	if actual.Type != expected.Type ||
		!slices.Equal(actual.Extensions, expected.Extensions) {
		return nil, fmt.Errorf(
			"MIME type %q extensions are %v; expected %v",
			expected.Type,
			actual.Extensions,
			expected.Extensions,
		)
	}
	if apiMIMEType.Origin != "user" {
		return nil, fmt.Errorf(
			"MIME type %q origin is %q; expected user",
			expected.Type,
			apiMIMEType.Origin,
		)
	}

	return apiMIMEType, nil
}

func (r *mimeTypeResource) rollbackCreatedMIMEType(
	ctx context.Context,
	attempted mimeTypeApplyTrace,
) error {
	current, err := r.client.Get(ctx, attempted.definition.Type)
	if err != nil {
		return fmt.Errorf("read created MIME type before rollback: %w", err)
	}
	if current == nil {
		return nil
	}
	if !attempted.matches(current) {
		return fmt.Errorf(
			"refuse to roll back MIME type creation because the current entry is not an attributable application prefix",
		)
	}

	return r.deleteMIMETypeDefinition(
		ctx,
		mimeTypeDefinitionFromAPI(*current),
		nil,
		"delete the created MIME type",
	)
}

func (r *mimeTypeResource) restoreMIMEType(
	ctx context.Context,
	attempted mimeTypeApplyTrace,
	original mimetype.Definition,
) error {
	current, err := r.client.Get(ctx, attempted.definition.Type)
	if err != nil {
		return fmt.Errorf("read replacement MIME type before rollback: %w", err)
	}
	if current != nil {
		if mimeTypeMatchesDefinition(current, original) {
			_, err := r.verifyMIMEType(ctx, original)
			if err != nil {
				return fmt.Errorf("verify restored MIME type: %w", err)
			}

			return nil
		}
		if !attempted.matches(current) {
			return fmt.Errorf(
				"refuse to restore the previous MIME type because the current replacement is not an attributable application prefix",
			)
		}
		if err := r.deleteMIMETypeDefinition(
			ctx,
			mimeTypeDefinitionFromAPI(*current),
			&original,
			"delete the replacement MIME type",
		); err != nil {
			return err
		}

		current, err = r.client.Get(ctx, original.Type)
		if err != nil {
			return fmt.Errorf(
				"read MIME type after deleting replacement: %w",
				err,
			)
		}
		if current != nil {
			if !mimeTypeMatchesDefinition(current, original) {
				return fmt.Errorf(
					"refuse to restore the previous MIME type because a concurrent definition appeared after deleting the replacement",
				)
			}
			if _, err := r.verifyMIMEType(ctx, original); err != nil {
				return fmt.Errorf("verify restored MIME type: %w", err)
			}

			return nil
		}
	}

	restoreTrace, err := r.applyDefinition(ctx, original)
	if err != nil {
		current, readErr := r.client.Get(ctx, original.Type)
		if readErr != nil {
			return fmt.Errorf(
				"restore previous MIME type: %v; read MIME type after ambiguous restore: %w",
				err,
				readErr,
			)
		}
		if mimeTypeMatchesDefinition(current, original) {
			return nil
		}
		if current != nil && !restoreTrace.matches(current) {
			return fmt.Errorf(
				"restore previous MIME type: %v; refuse to overwrite a concurrent MIME type definition",
				err,
			)
		}

		return fmt.Errorf("restore previous MIME type: %w", err)
	}
	if _, err := r.verifyMIMEType(ctx, original); err != nil {
		return fmt.Errorf("verify restored MIME type: %w", err)
	}

	return nil
}

func (r *mimeTypeResource) deleteMIMETypeDefinition(
	ctx context.Context,
	expected mimetype.Definition,
	alreadyRestored *mimetype.Definition,
	operation string,
) error {
	current, err := r.client.Get(ctx, expected.Type)
	if err != nil {
		return fmt.Errorf("read MIME type before %s: %w", operation, err)
	}
	if current == nil {
		return nil
	}
	if alreadyRestored != nil &&
		mimeTypeMatchesDefinition(current, *alreadyRestored) {
		return nil
	}
	if !mimeTypeMatchesDefinition(current, expected) {
		return fmt.Errorf(
			"refuse to %s because the current MIME type no longer matches the expected definition",
			operation,
		)
	}

	deleteErr := r.client.Delete(ctx, expected.Type)
	remaining, readErr := r.client.Get(ctx, expected.Type)
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
		mimeTypeMatchesDefinition(remaining, *alreadyRestored) {
		return nil
	}
	if mimeTypeMatchesDefinition(remaining, expected) {
		if deleteErr != nil {
			return fmt.Errorf("%s: %w", operation, deleteErr)
		}

		return fmt.Errorf(
			"%s reported success but the expected MIME type still exists",
			operation,
		)
	}

	return fmt.Errorf(
		"refuse to continue after %s because a concurrent MIME type definition is now present",
		operation,
	)
}

func (trace *mimeTypeApplyTrace) addAttributablePrefix(
	prefix mimetype.Definition,
) {
	trace.attributablePrefixes = append(
		trace.attributablePrefixes,
		prefix.Sorted(),
	)
}

func (trace mimeTypeApplyTrace) matches(current *mimetype.MIMEType) bool {
	if current == nil || current.Origin != "user" {
		return false
	}

	currentDefinition := mimeTypeDefinitionFromAPI(*current)
	for _, prefix := range trace.attributablePrefixes {
		if mimeTypeDefinitionsEqual(currentDefinition, prefix) {
			return true
		}
	}

	return false
}

func mimeTypeMatchesDefinition(
	mimeType *mimetype.MIMEType,
	expected mimetype.Definition,
) bool {
	return mimeType != nil &&
		mimeType.Origin == "user" &&
		mimeTypeDefinitionsEqual(
			mimeTypeDefinitionFromAPI(*mimeType),
			expected,
		)
}

func mimeTypeDefinitionsEqual(
	left mimetype.Definition,
	right mimetype.Definition,
) bool {
	left = left.Sorted()
	right = right.Sorted()

	return left.Type == right.Type &&
		slices.Equal(left.Extensions, right.Extensions)
}

func mimeTypeMutationErrorDetail(primaryError, rollbackError error) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the MIME type change: %v",
		primaryError,
		rollbackError,
	)
}
