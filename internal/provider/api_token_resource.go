package provider

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/apitoken"
)

var (
	_ resource.Resource                = &apiTokenResource{}
	_ resource.ResourceWithConfigure   = &apiTokenResource{}
	_ resource.ResourceWithImportState = &apiTokenResource{}
)

func NewAPITokenResource() resource.Resource {
	return &apiTokenResource{}
}

type apiTokenResource struct {
	client *apitoken.Client
}

func (r *apiTokenResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_api_token"
}

func (r *apiTokenResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a full-access cPanel API token and its optional expiration.",
		MarkdownDescription: "Manages a full-access cPanel API token and its optional expiration.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The API token name. Renaming updates the existing token without rotating its secret.",
				MarkdownDescription: "The API token name. Renaming updates the existing token without rotating its secret.",
				Validators:          apiTokenNameValidators(),
			},
			"expires_at": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Description:         "The future Unix timestamp when the token expires, or 0 for no expiration. Changing it rotates the token.",
				MarkdownDescription: "The future Unix timestamp when the token expires, or `0` for no expiration. Changing it rotates the token.",
				Validators:          apiTokenExpirationValidators(),
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"token": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The API token secret. cPanel returns it only during creation, so imported resources cannot recover it.",
				MarkdownDescription: "The API token secret. cPanel returns it only during creation, so imported resources cannot recover it.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The token creation time as a Unix timestamp.",
				MarkdownDescription: "The token creation time as a Unix timestamp.",
			},
			"has_full_access": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports that the token has full account access.",
				MarkdownDescription: "Whether cPanel reports that the token has full account access.",
			},
			"features": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The feature names attached to a limited token imported from cPanel. Newly created tokens have full access.",
				MarkdownDescription: "The feature names attached to a limited token imported from cPanel. Newly created tokens have full access.",
			},
			"whitelist_ips": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The source IP restrictions reported by cPanel.",
				MarkdownDescription: "The source IP restrictions reported by cPanel.",
			},
		},
	}
}

func (r *apiTokenResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state APITokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	token, err := r.client.Get(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API token", err.Error())
		return
	}
	if token == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(applyAPITokenToResourceModel(ctx, &state, *token)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *apiTokenResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan APITokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := plan.Name.ValueString()
	expiresAt := plan.ExpiresAt.ValueInt64()
	if err := validateAPITokenExpiration(expiresAt, time.Now().Unix()); err != nil {
		resp.Diagnostics.AddError("Invalid API token expiration", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API token", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"API token already exists",
			fmt.Sprintf(
				"An API token named %q already exists. Import it instead of replacing it implicitly.",
				name,
			),
		)
		return
	}

	created, err := r.client.Create(ctx, name, expiresAt)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create API token",
			"Could not create API token: "+err.Error(),
		)
		return
	}

	token, err := r.verifyToken(ctx, name, expiresAt, true)
	if err != nil {
		rollbackErr := r.client.Revoke(ctx, name)
		resp.Diagnostics.AddError(
			"Unable to verify API token",
			apiTokenMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	plan.Token = types.StringValue(created.Token)
	metadataDiagnostics := applyAPITokenToResourceModel(ctx, &plan, *token)
	resp.Diagnostics.Append(metadataDiagnostics...)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.client.Revoke(ctx, name)
		resp.Diagnostics.AddError(
			"Unable to decode API token",
			apiTokenMutationErrorDetail(
				fmt.Errorf("convert API token metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan APITokenResourceModel
	var state APITokenResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	oldName := state.Name.ValueString()
	newName := plan.Name.ValueString()
	if oldName == newName {
		resp.Diagnostics.AddError(
			"Unsupported API token update",
			"Only the API token name can be updated in place.",
		)
		return
	}

	current, err := r.client.Get(ctx, oldName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API token", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"API token no longer exists",
			"Refresh the Terraform state before updating the API token.",
		)
		return
	}

	existing, err := r.client.Get(ctx, newName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read target API token name", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"API token name already exists",
			fmt.Sprintf("An API token named %q already exists.", newName),
		)
		return
	}

	if err := r.client.Rename(ctx, oldName, newName); err != nil {
		resp.Diagnostics.AddError(
			"Unable to rename API token",
			"Could not rename API token: "+err.Error(),
		)
		return
	}

	token, err := r.verifyToken(
		ctx,
		newName,
		state.ExpiresAt.ValueInt64(),
		false,
	)
	if err != nil {
		rollbackErr := r.client.Rename(ctx, newName, oldName)
		resp.Diagnostics.AddError(
			"Unable to verify API token rename",
			apiTokenMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	plan.Token = state.Token
	resp.Diagnostics.Append(applyAPITokenToResourceModel(ctx, &plan, *token)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *apiTokenResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state APITokenResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !state.Token.IsNull() &&
		!state.Token.IsUnknown() &&
		state.Token.ValueString() == r.client.Auth.APIToken {
		resp.Diagnostics.AddError(
			"Refusing to revoke active provider token",
			"The API token being deleted is also configuring this provider. Configure the provider with a different token before destroying this resource.",
		)
		return
	}

	name := state.Name.ValueString()
	existing, err := r.client.Get(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API token", err.Error())
		return
	}
	if existing == nil {
		return
	}

	if err := r.client.Revoke(ctx, name); err != nil {
		resp.Diagnostics.AddError(
			"Unable to revoke API token",
			"Could not revoke API token: "+err.Error(),
		)
		return
	}

	remaining, err := r.client.Get(ctx, name)
	if err != nil {
		resp.Diagnostics.AddError("Unable to verify API token revocation", err.Error())
		return
	}
	if remaining != nil {
		resp.Diagnostics.AddError(
			"Unable to verify API token revocation",
			fmt.Sprintf("API token %q still exists after revocation.", name),
		)
	}
}

func (r *apiTokenResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...,
	)
}

func (r *apiTokenResource) Configure(
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

	client, ok := providerData["apitoken"].(*apitoken.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected API Token Client Type",
			fmt.Sprintf("Expected *apitoken.Client, got: %T.", providerData["apitoken"]),
		)
		return
	}

	r.client = client
}

func (r *apiTokenResource) verifyToken(
	ctx context.Context,
	name string,
	expectedExpiresAt int64,
	requireFullAccess bool,
) (*apitoken.Token, error) {
	token, err := r.client.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("read API token after mutation: %w", err)
	}
	if token == nil {
		return nil, fmt.Errorf("API token %q was not found after mutation", name)
	}
	if token.ExpiresAt.ValueOrZero() != expectedExpiresAt {
		return nil, fmt.Errorf(
			"API token %q expires at %d; expected %d",
			name,
			token.ExpiresAt.ValueOrZero(),
			expectedExpiresAt,
		)
	}
	if requireFullAccess && token.HasFullAccess != 1 {
		return nil, fmt.Errorf(
			"API token %q does not have full access after creation",
			name,
		)
	}

	return token, nil
}

func apiTokenMutationErrorDetail(primaryError, rollbackError error) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the API token change: %v",
		primaryError,
		rollbackError,
	)
}
