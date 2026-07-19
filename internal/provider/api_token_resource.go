package provider

import (
	"context"
	"fmt"
	"slices"
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
	client          *apitoken.Client
	activeTokenName string
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
				Description:         "The API token name. Renaming updates a non-active token without rotating its secret.",
				MarkdownDescription: "The API token name. Renaming updates a non-active token without rotating its secret.",
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
		if created != nil && created.CreateTime != 0 {
			rollbackErr := r.rollbackCreatedToken(
				ctx,
				name,
				created.CreateTime,
			)
			resp.Diagnostics.AddError(
				"Unable to create API token",
				apiTokenMutationErrorDetail(
					err,
					rollbackErr,
				),
			)
			return
		}
		if !cPanelMutationErrorIsDeterministic(err) {
			token, recoveryErr := r.client.Get(ctx, name)
			switch {
			case recoveryErr != nil:
				resp.Diagnostics.AddError(
					"Unable to reconcile API token creation",
					fmt.Sprintf(
						"%v. Terraform could not determine whether cPanel created the token because the follow-up inventory read also failed: %v",
						err,
						recoveryErr,
					),
				)
			case token != nil:
				resp.Diagnostics.AddError(
					"API token secret is unavailable",
					fmt.Sprintf(
						"cPanel may have created API token %q, but the creation response did not return a usable secret. Terraform refuses to adopt a token whose secret cannot be recovered. Revoke the token in cPanel before retrying.",
						name,
					),
				)
			default:
				resp.Diagnostics.AddError(
					"Unable to create API token",
					"Could not create API token: "+err.Error(),
				)
			}
			return
		}
		resp.Diagnostics.AddError(
			"Unable to create API token",
			"Could not create API token: "+err.Error(),
		)
		return
	}

	token, err := r.verifyToken(ctx, name, expiresAt, true)
	if err != nil {
		rollbackErr := r.rollbackCreatedToken(
			ctx,
			name,
			created.CreateTime,
		)
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
		rollbackErr := r.rollbackCreatedToken(
			ctx,
			name,
			created.CreateTime,
		)
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
	if err := validateAPITokenRename(
		state,
		r.client.Auth.APIToken,
		r.activeTokenName,
	); err != nil {
		resp.Diagnostics.AddError(
			"Refusing to rename active provider token",
			err.Error(),
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
	matchesState, err := apiTokenMatchesState(ctx, *current, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify API token identity",
			err.Error(),
		)
		return
	}
	if !matchesState {
		resp.Diagnostics.AddError(
			"API token changed during update",
			fmt.Sprintf(
				"API token %q no longer matches the full observable identity stored in Terraform state. Refresh and review the change before retrying.",
				oldName,
			),
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
		if cPanelMutationErrorIsDeterministic(err) {
			resp.Diagnostics.AddError(
				"Unable to rename API token",
				"Could not rename API token: "+err.Error(),
			)
			return
		}

		applied, recoveryErr := r.reconcileTokenRename(
			ctx,
			oldName,
			newName,
			*current,
		)
		if recoveryErr != nil {
			resp.Diagnostics.AddError(
				"Unable to reconcile API token rename",
				fmt.Sprintf(
					"%v. Read-only recovery could not determine whether the rename was applied: %v",
					err,
					recoveryErr,
				),
			)
			return
		}
		if !applied {
			resp.Diagnostics.AddError(
				"Unable to rename API token",
				"Could not rename API token: "+err.Error(),
			)
			return
		}
	}

	token, err := r.verifyToken(
		ctx,
		newName,
		state.ExpiresAt.ValueInt64(),
		false,
	)
	if err != nil {
		rollbackErr := r.rollbackTokenRename(
			ctx,
			*current,
			oldName,
			newName,
		)
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

func (r *apiTokenResource) rollbackCreatedToken(
	ctx context.Context,
	name string,
	expectedCreateTime int64,
) error {
	token, err := r.client.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("read API token before rollback: %w", err)
	}
	if token == nil {
		return nil
	}
	if expectedCreateTime == 0 || token.CreateTime != expectedCreateTime {
		return fmt.Errorf(
			"refuse to revoke API token %q because its creation time no longer matches the token created by Terraform",
			name,
		)
	}

	return r.client.Revoke(ctx, name)
}

func (r *apiTokenResource) reconcileTokenRename(
	ctx context.Context,
	oldName string,
	newName string,
	original apitoken.Token,
) (bool, error) {
	oldToken, err := r.client.Get(ctx, oldName)
	if err != nil {
		return false, fmt.Errorf("read old API token: %w", err)
	}
	newToken, err := r.client.Get(ctx, newName)
	if err != nil {
		return false, fmt.Errorf("read new API token: %w", err)
	}
	switch {
	case oldToken != nil && newToken == nil:
		return false, nil
	case oldToken != nil && newToken != nil:
		return false, fmt.Errorf(
			"both API token names exist after the rename request",
		)
	case oldToken == nil && newToken == nil:
		return false, fmt.Errorf(
			"neither API token name exists after the rename request",
		)
	}
	if !apiTokensHaveSameIdentity(original, *newToken) {
		return false, fmt.Errorf(
			"API token %q does not match the token previously managed as %q",
			newName,
			oldName,
		)
	}

	return true, nil
}

func (r *apiTokenResource) rollbackTokenRename(
	ctx context.Context,
	original apitoken.Token,
	oldName string,
	newName string,
) error {
	oldToken, err := r.client.Get(ctx, oldName)
	if err != nil {
		return fmt.Errorf("read original API token name before rollback: %w", err)
	}
	newToken, err := r.client.Get(ctx, newName)
	if err != nil {
		return fmt.Errorf("read renamed API token before rollback: %w", err)
	}
	if oldToken != nil && newToken == nil {
		return nil
	}
	if oldToken != nil || newToken == nil ||
		!apiTokensHaveSameIdentity(original, *newToken) {
		return fmt.Errorf(
			"refuse to restore API token name because the current token inventory no longer matches the attempted rename",
		)
	}

	return r.client.Rename(ctx, newName, oldName)
}

func apiTokensHaveSameIdentity(left, right apitoken.Token) bool {
	leftFeatures := append([]string(nil), left.Features...)
	rightFeatures := append([]string(nil), right.Features...)
	leftWhitelistIPs := append([]string(nil), left.WhitelistIPs...)
	rightWhitelistIPs := append([]string(nil), right.WhitelistIPs...)
	slices.Sort(leftFeatures)
	slices.Sort(rightFeatures)
	slices.Sort(leftWhitelistIPs)
	slices.Sort(rightWhitelistIPs)

	return left.CreateTime == right.CreateTime &&
		left.ExpiresAt == right.ExpiresAt &&
		left.HasFullAccess == right.HasFullAccess &&
		slices.Equal(leftFeatures, rightFeatures) &&
		slices.Equal(leftWhitelistIPs, rightWhitelistIPs)
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

	if err := validateAPITokenDeletion(
		state,
		r.client.Auth.APIToken,
		r.activeTokenName,
	); err != nil {
		resp.Diagnostics.AddError(
			"Refusing to revoke active provider token",
			err.Error(),
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
	matchesState, err := apiTokenMatchesState(ctx, *existing, state)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify API token identity",
			err.Error(),
		)
		return
	}
	if !matchesState {
		resp.Diagnostics.AddError(
			"Refusing to revoke changed API token",
			fmt.Sprintf(
				"API token %q no longer matches the full identity stored in Terraform state, including its creation time. Refresh and review the replacement before retrying.",
				name,
			),
		)
		return
	}

	revokeErr := r.client.Revoke(ctx, name)
	remaining, verifyErr := r.client.Get(ctx, name)
	if verifyErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify API token revocation",
			fmt.Sprintf(
				"Revoke response: %v. Follow-up read failed: %v",
				revokeErr,
				verifyErr,
			),
		)
		return
	}
	if remaining == nil {
		return
	}

	remainingMatchesState, matchErr := apiTokenMatchesState(
		ctx,
		*remaining,
		state,
	)
	if matchErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify API token revocation",
			matchErr.Error(),
		)
		return
	}
	if !remainingMatchesState {
		resp.Diagnostics.AddError(
			"API token replacement preserved",
			fmt.Sprintf(
				"API token %q exists after the revocation attempt but no longer matches the token stored in Terraform state. Terraform will not revoke the replacement.",
				name,
			),
		)
		return
	}
	if revokeErr != nil {
		resp.Diagnostics.AddError(
			"Unable to revoke API token",
			"Could not revoke API token: "+revokeErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to verify API token revocation",
		fmt.Sprintf(
			"API token %q still exists after cPanel reported a successful revocation.",
			name,
		),
	)
}

func apiTokenMatchesState(
	ctx context.Context,
	token apitoken.Token,
	state APITokenResourceModel,
) (bool, error) {
	if state.Name.IsNull() || state.Name.IsUnknown() ||
		state.ExpiresAt.IsNull() || state.ExpiresAt.IsUnknown() ||
		state.CreatedAt.IsNull() || state.CreatedAt.IsUnknown() ||
		state.HasFullAccess.IsNull() || state.HasFullAccess.IsUnknown() ||
		state.Features.IsNull() || state.Features.IsUnknown() ||
		state.WhitelistIPs.IsNull() || state.WhitelistIPs.IsUnknown() {
		return false, fmt.Errorf(
			"API token state does not contain a complete observable identity; refresh the state before mutating the token",
		)
	}

	var features []string
	diagnostics := state.Features.ElementsAs(ctx, &features, false)
	if diagnostics.HasError() {
		return false, fmt.Errorf(
			"decode API token features from state: %v",
			diagnostics,
		)
	}
	var whitelistIPs []string
	diagnostics = state.WhitelistIPs.ElementsAs(
		ctx,
		&whitelistIPs,
		false,
	)
	if diagnostics.HasError() {
		return false, fmt.Errorf(
			"decode API token IP restrictions from state: %v",
			diagnostics,
		)
	}

	expectedAccess := 0
	if state.HasFullAccess.ValueBool() {
		expectedAccess = 1
	}
	slices.Sort(features)
	slices.Sort(whitelistIPs)
	tokenFeatures := append([]string(nil), token.Features...)
	tokenWhitelistIPs := append([]string(nil), token.WhitelistIPs...)
	slices.Sort(tokenFeatures)
	slices.Sort(tokenWhitelistIPs)

	return token.Name == state.Name.ValueString() &&
		token.CreateTime == state.CreatedAt.ValueInt64() &&
		token.ExpiresAt.ValueOrZero() == state.ExpiresAt.ValueInt64() &&
		token.HasFullAccess == expectedAccess &&
		slices.Equal(tokenFeatures, features) &&
		slices.Equal(tokenWhitelistIPs, whitelistIPs), nil
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
	activeTokenName, ok := providerData["api_token_name"].(string)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected API Token Name Type",
			fmt.Sprintf(
				"Expected string, got: %T.",
				providerData["api_token_name"],
			),
		)
		return
	}
	r.activeTokenName = activeTokenName
}

func validateAPITokenDeletion(
	state APITokenResourceModel,
	activeTokenSecret string,
	activeTokenName string,
) error {
	return validateAPITokenMutation(
		state,
		activeTokenSecret,
		activeTokenName,
		"destroying",
	)
}

func validateAPITokenRename(
	state APITokenResourceModel,
	activeTokenSecret string,
	activeTokenName string,
) error {
	return validateAPITokenMutation(
		state,
		activeTokenSecret,
		activeTokenName,
		"renaming",
	)
}

func validateAPITokenMutation(
	state APITokenResourceModel,
	activeTokenSecret string,
	activeTokenName string,
	action string,
) error {
	name := state.Name.ValueString()
	if activeTokenName != "" && name == activeTokenName {
		return fmt.Errorf(
			"API token %q is declared as the token configuring this provider; configure the provider with a different token and update api_token_name or CPANEL_API_TOKEN_NAME before %s this resource",
			name,
			action,
		)
	}

	if !state.Token.IsNull() && !state.Token.IsUnknown() {
		if state.Token.ValueString() == activeTokenSecret {
			return fmt.Errorf(
				"API token %q is also configuring this provider; configure the provider with a different token before %s this resource",
				name,
				action,
			)
		}

		return nil
	}

	if activeTokenName == "" {
		return fmt.Errorf(
			"API token %q was imported, so cPanel cannot return its secret and Terraform cannot determine whether it configures this provider; configure api_token_name or CPANEL_API_TOKEN_NAME with the active provider token name before %s this resource",
			name,
			action,
		)
	}

	return nil
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
