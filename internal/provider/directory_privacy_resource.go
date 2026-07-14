package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/directoryprivacy"
)

var (
	_ resource.Resource                = &directoryPrivacyResource{}
	_ resource.ResourceWithConfigure   = &directoryPrivacyResource{}
	_ resource.ResourceWithImportState = &directoryPrivacyResource{}
)

func NewDirectoryPrivacyResource() resource.Resource {
	return &directoryPrivacyResource{}
}

type directoryPrivacyResource struct {
	client *directoryprivacy.Client
}

func (r *directoryPrivacyResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_directory_privacy"
}

func (r *directoryPrivacyResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Enables password protection for an existing directory in a cPanel account.",
		MarkdownDescription: "Enables password protection for an existing directory in a cPanel account.",
		Attributes: map[string]schema.Attribute{
			"directory": schema.StringAttribute{
				Required:            true,
				Description:         "The directory relative to the cPanel account home.",
				MarkdownDescription: "The normalized directory path relative to the cPanel account home.",
				Validators:          directoryIndexDirectoryValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"auth_name": schema.StringAttribute{
				Required:            true,
				Description:         "The label displayed by HTTP Basic authentication.",
				MarkdownDescription: "The label displayed by HTTP Basic authentication.",
				Validators:          directoryPrivacyAuthNameValidators(),
			},
			"absolute_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute directory path reported by cPanel.",
				MarkdownDescription: "The absolute directory path reported by cPanel.",
			},
			"auth_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The authentication type reported by cPanel.",
				MarkdownDescription: "The authentication type reported by cPanel. Managed protection uses `Basic`.",
			},
			"password_file": schema.StringAttribute{
				Computed:            true,
				Description:         "The password file path reported by cPanel.",
				MarkdownDescription: "The password file path reported by cPanel.",
			},
			"protected": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the directory as protected.",
				MarkdownDescription: "Whether cPanel reports the directory as protected.",
			},
		},
	}
}

func (r *directoryPrivacyResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DirectoryPrivacyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiPrivacy, err := r.client.Get(ctx, state.Directory.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if apiPrivacy == nil || !apiPrivacy.Protected {
		resp.State.RemoveResource(ctx)
		return
	}

	applyDirectoryPrivacyToResourceModel(&state, *apiPrivacy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *directoryPrivacyResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DirectoryPrivacyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := directoryPrivacyDefinitionFromResourceModel(plan)
	if err := validateDirectoryPrivacyDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid directory privacy", err.Error())
		return
	}

	current, err := r.client.Get(ctx, definition.Directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Directory not found",
			fmt.Sprintf(
				"Directory %q does not exist in the cPanel account.",
				definition.Directory,
			),
		)
		return
	}
	if current.Protected {
		resp.Diagnostics.AddError(
			"Directory privacy already exists",
			fmt.Sprintf(
				"Directory %q is already protected. Import it instead of replacing it implicitly.",
				definition.Directory,
			),
		)
		return
	}

	if _, err := r.client.Configure(
		ctx,
		definition.Directory,
		definition.AuthName,
		true,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to enable directory privacy",
			createMutationErrorDetail(
				err,
				"directory privacy creation",
				fmt.Sprintf(
					"the directory privacy setting for %q",
					definition.Directory,
				),
			),
		)
		return
	}

	apiPrivacy, err := r.verifyDirectoryPrivacy(ctx, definition)
	if err != nil {
		rollbackErr := r.disableDirectoryPrivacyIfCurrentMatches(
			ctx,
			definition,
		)
		resp.Diagnostics.AddError(
			"Unable to verify directory privacy",
			directoryPrivacyMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyDirectoryPrivacyToResourceModel(&plan, *apiPrivacy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryPrivacyResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DirectoryPrivacyResourceModel
	var state DirectoryPrivacyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	definition := directoryPrivacyDefinitionFromResourceModel(plan)
	if err := validateDirectoryPrivacyDefinition(definition); err != nil {
		resp.Diagnostics.AddError("Invalid directory privacy", err.Error())
		return
	}

	current, err := r.client.Get(ctx, state.Directory.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if current == nil || !current.Protected {
		resp.Diagnostics.AddError(
			"Directory privacy no longer exists",
			"Refresh the Terraform state before updating directory privacy.",
		)
		return
	}
	if current.AuthName != state.AuthName.ValueString() {
		resp.Diagnostics.AddError(
			"Directory privacy changed outside Terraform",
			"The remote directory privacy configuration no longer matches Terraform state. Refresh and review the drift before retrying.",
		)
		return
	}

	changed := current.AuthName != definition.AuthName
	if changed {
		if _, err := r.client.Configure(
			ctx,
			definition.Directory,
			definition.AuthName,
			true,
		); err != nil {
			rollbackErr := r.restoreDirectoryPrivacyIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
			resp.Diagnostics.AddError(
				"Unable to update directory privacy",
				directoryPrivacyMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	apiPrivacy, err := r.verifyDirectoryPrivacy(ctx, definition)
	if err != nil {
		var rollbackErr error
		if changed {
			rollbackErr = r.restoreDirectoryPrivacyIfCurrentMatches(
				ctx,
				definition,
				*current,
			)
		}
		resp.Diagnostics.AddError(
			"Unable to verify directory privacy update",
			directoryPrivacyMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyDirectoryPrivacyToResourceModel(&plan, *apiPrivacy)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *directoryPrivacyResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DirectoryPrivacyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	directory := state.Directory.ValueString()
	current, err := r.client.Get(ctx, directory)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read directory privacy",
			err.Error(),
		)
		return
	}
	if current == nil || !current.Protected {
		return
	}
	if current.AuthName != state.AuthName.ValueString() {
		resp.Diagnostics.AddError(
			"Unable to disable directory privacy",
			"The remote directory privacy configuration no longer matches Terraform state, so the provider refuses to disable it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.disableDirectoryPrivacy(ctx, directory); err != nil {
		resp.Diagnostics.AddError(
			"Unable to disable directory privacy",
			err.Error(),
		)
	}
}

func (r *directoryPrivacyResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := validateDirectoryPrivacyDefinition(
		directoryprivacy.Definition{
			Directory: req.ID,
			AuthName:  "import",
		},
	); err != nil {
		resp.Diagnostics.AddError(
			"Invalid directory privacy import identifier",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("directory"), req.ID)...,
	)
}

func (r *directoryPrivacyResource) Configure(
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

	client, ok := providerData["directoryprivacy"].(*directoryprivacy.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Directory Privacy Client Type",
			fmt.Sprintf(
				"Expected *directoryprivacy.Client, got: %T.",
				providerData["directoryprivacy"],
			),
		)
		return
	}

	r.client = client
}

func (r *directoryPrivacyResource) verifyDirectoryPrivacy(
	ctx context.Context,
	expected directoryprivacy.Definition,
) (*directoryprivacy.Privacy, error) {
	apiPrivacy, err := r.client.Get(ctx, expected.Directory)
	if err != nil {
		return nil, fmt.Errorf("read directory privacy after mutation: %w", err)
	}
	if apiPrivacy == nil {
		return nil, fmt.Errorf(
			"directory %q was not found after privacy mutation",
			expected.Directory,
		)
	}
	if !apiPrivacy.Protected {
		return nil, fmt.Errorf(
			"directory %q is not protected after privacy mutation",
			expected.Directory,
		)
	}
	if apiPrivacy.AuthName != expected.AuthName {
		return nil, fmt.Errorf(
			"directory %q auth name is %q; expected %q",
			expected.Directory,
			apiPrivacy.AuthName,
			expected.AuthName,
		)
	}

	return apiPrivacy, nil
}

func (r *directoryPrivacyResource) disableDirectoryPrivacyIfCurrentMatches(
	ctx context.Context,
	attempted directoryprivacy.Definition,
) error {
	current, err := r.client.Get(ctx, attempted.Directory)
	if err != nil {
		return fmt.Errorf("read directory privacy before rollback: %w", err)
	}
	if current == nil || !current.Protected {
		return nil
	}
	if current.AuthName != attempted.AuthName {
		return fmt.Errorf(
			"refuse to roll back directory privacy creation because the current configuration no longer matches the attempted definition",
		)
	}

	return r.disableDirectoryPrivacy(ctx, attempted.Directory)
}

func (r *directoryPrivacyResource) disableDirectoryPrivacy(
	ctx context.Context,
	directory string,
) error {
	_, mutationErr := r.client.Configure(ctx, directory, "", false)
	apiPrivacy, readErr := r.client.Get(ctx, directory)
	if readErr != nil {
		if mutationErr != nil {
			return fmt.Errorf(
				"disable directory privacy: %v; verify disabled directory privacy: %w",
				mutationErr,
				readErr,
			)
		}

		return fmt.Errorf("verify disabled directory privacy: %w", readErr)
	}
	if apiPrivacy == nil {
		return nil
	}
	if !apiPrivacy.Protected {
		return nil
	}
	if mutationErr != nil {
		return fmt.Errorf("disable directory privacy: %w", mutationErr)
	}

	return fmt.Errorf(
		"directory %q is still protected after disabling privacy",
		directory,
	)
}

func (r *directoryPrivacyResource) restoreDirectoryPrivacyIfCurrentMatches(
	ctx context.Context,
	expectedCurrent directoryprivacy.Definition,
	original directoryprivacy.Privacy,
) error {
	current, err := r.client.Get(ctx, original.Directory)
	if err != nil {
		return fmt.Errorf("read directory privacy before restore: %w", err)
	}
	if current == nil {
		return fmt.Errorf(
			"directory %q no longer exists during directory privacy restore",
			original.Directory,
		)
	}
	if current.Protected && current.AuthName == original.AuthName {
		return nil
	}
	if !current.Protected || current.AuthName != expectedCurrent.AuthName {
		return fmt.Errorf(
			"refuse to restore the previous directory privacy because the current configuration no longer matches the Terraform transition",
		)
	}
	if _, err := r.client.Configure(
		ctx,
		original.Directory,
		original.AuthName,
		true,
	); err != nil {
		return fmt.Errorf("restore previous directory privacy: %w", err)
	}
	if _, err := r.verifyDirectoryPrivacy(
		ctx,
		directoryprivacy.Definition{
			Directory: original.Directory,
			AuthName:  original.AuthName,
		},
	); err != nil {
		return fmt.Errorf("verify restored directory privacy: %w", err)
	}

	return nil
}

func directoryPrivacyMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous directory privacy state: %v",
		primaryError,
		rollbackError,
	)
}
