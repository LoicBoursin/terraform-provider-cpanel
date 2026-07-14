package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ddns"
	cpaneldomain "terraform-provider-cpanel/internal/cpanel/domain"
)

var (
	_ resource.Resource                = &dynamicDNSResource{}
	_ resource.ResourceWithConfigure   = &dynamicDNSResource{}
	_ resource.ResourceWithImportState = &dynamicDNSResource{}
)

func NewDynamicDNSResource() resource.Resource {
	return &dynamicDNSResource{}
}

type dynamicDNSResource struct {
	client       dynamicDNSClient
	domainClient *cpaneldomain.Client
}

type dynamicDNSClient interface {
	Get(context.Context, string) (*ddns.Domain, error)
	Create(context.Context, string, string) (*ddns.CreatedDomain, error)
	SetDescription(context.Context, string, string) error
	Delete(context.Context, string) (bool, error)
}

func (r *dynamicDNSResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_dynamic_dns"
}

func (r *dynamicDNSResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages a cPanel Dynamic DNS domain and its webcall URL.",
		MarkdownDescription: "Manages a cPanel Dynamic DNS domain and its sensitive webcall URL.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The complete Dynamic DNS subdomain under an existing main or addon domain.",
				MarkdownDescription: "The complete Dynamic DNS subdomain under an existing main or addon domain.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				Description:         "A human-readable description of the Dynamic DNS domain.",
				MarkdownDescription: "A human-readable description of the Dynamic DNS domain.",
			},
			"webcall_id": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The secret identifier used by the Dynamic DNS update endpoint.",
				MarkdownDescription: "The secret identifier used by the Dynamic DNS update endpoint.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"webcall_url": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				Description:         "The HTTPS URL that a router or device calls to update the domain addresses.",
				MarkdownDescription: "The HTTPS URL that a router or device calls to update the domain addresses.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created_at": schema.Int64Attribute{
				Computed:            true,
				Description:         "The Dynamic DNS domain creation time as a Unix timestamp.",
				MarkdownDescription: "The Dynamic DNS domain creation time as a Unix timestamp.",
			},
			"ipv4": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The IPv4 addresses currently associated with the Dynamic DNS domain.",
				MarkdownDescription: "The IPv4 addresses currently associated with the Dynamic DNS domain.",
			},
			"ipv6": schema.SetAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The IPv6 addresses currently associated with the Dynamic DNS domain.",
				MarkdownDescription: "The IPv6 addresses currently associated with the Dynamic DNS domain.",
			},
			"last_run_times": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.Int64Type,
				Description:         "The most recent webcall execution times as Unix timestamps.",
				MarkdownDescription: "The most recent webcall execution times as Unix timestamps.",
			},
			"last_update_time": schema.Int64Attribute{
				Computed:            true,
				Description:         "The most recent address-change time as a Unix timestamp.",
				MarkdownDescription: "The most recent address-change time as a Unix timestamp.",
			},
		},
	}
}

func (r *dynamicDNSResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DynamicDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain, err := r.client.Get(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Dynamic DNS domain", err.Error())
		return
	}
	if domain == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(applyDynamicDNSToResourceModel(ctx, &state, *domain)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dynamicDNSResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DynamicDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := plan.Domain.ValueString()
	if _, _, err := resolveSubdomainParts(
		ctx,
		r.domainClient,
		domainName,
	); err != nil {
		resp.Diagnostics.AddError("Invalid Dynamic DNS domain", err.Error())
		return
	}

	existing, err := r.client.Get(ctx, domainName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Dynamic DNS domain", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain already exists",
			fmt.Sprintf(
				"Dynamic DNS domain %q already exists. Import it instead of replacing it implicitly.",
				domainName,
			),
		)
		return
	}

	marker, err := newDynamicDNSCreationMarker()
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to create Dynamic DNS domain",
			err.Error(),
		)
		return
	}

	created, createErr := r.client.Create(ctx, domainName, marker)
	createdID := ""
	if createErr == nil {
		createdID = created.ID
	} else if cPanelMutationErrorIsDeterministic(createErr) {
		resp.Diagnostics.AddError(
			"Unable to create Dynamic DNS domain",
			dynamicDNSCredentialMutationError(
				createErr,
				"creation",
			).Error(),
		)
		return
	} else {
		reconciled, readErr := r.client.Get(ctx, domainName)
		if readErr != nil {
			resp.Diagnostics.AddError(
				"Unable to create Dynamic DNS domain",
				fmt.Sprintf(
					"%v. Terraform could not determine whether the marked Dynamic DNS domain was created: %v",
					dynamicDNSCredentialMutationError(createErr, "creation"),
					readErr,
				),
			)
			return
		}
		if reconciled == nil ||
			reconciled.ID == "" ||
			reconciled.Description != marker {
			resp.Diagnostics.AddError(
				"Unable to create Dynamic DNS domain",
				fmt.Sprintf(
					"%v. Terraform did not adopt any domain because the creation could not be attributed safely.",
					dynamicDNSCredentialMutationError(createErr, "creation"),
				),
			)
			return
		}
		createdID = reconciled.ID
	}

	description := plan.Description.ValueString()
	setErr := r.client.SetDescription(ctx, createdID, description)
	if setErr != nil {
		reconciled, applied, reconcileErr := r.reconcileDynamicDNSDescription(
			ctx,
			domainName,
			createdID,
			marker,
			description,
		)
		if reconcileErr != nil {
			rollbackErr := r.rollbackCreatedDynamicDNS(
				ctx,
				domainName,
				createdID,
				marker,
				description,
			)
			resp.Diagnostics.AddError(
				"Unable to set Dynamic DNS description",
				dynamicDNSMutationErrorDetail(
					fmt.Errorf(
						"%v. Terraform could not reconcile the description update: %w",
						dynamicDNSCredentialMutationError(setErr, "description update"),
						reconcileErr,
					),
					rollbackErr,
				),
			)
			return
		}
		if !applied {
			rollbackErr := r.rollbackCreatedDynamicDNS(
				ctx,
				domainName,
				createdID,
				marker,
			)
			resp.Diagnostics.AddError(
				"Unable to set Dynamic DNS description",
				dynamicDNSMutationErrorDetail(
					dynamicDNSCredentialMutationError(setErr, "description update"),
					rollbackErr,
				),
			)
			return
		}
		if reconciled == nil {
			resp.Diagnostics.AddError(
				"Unable to set Dynamic DNS description",
				"Terraform reconciled the description update without a Dynamic DNS domain.",
			)
			return
		}
	}

	domain, err := r.verifyDynamicDNS(
		ctx,
		domainName,
		description,
		createdID,
	)
	if err != nil {
		rollbackErr := r.rollbackCreatedDynamicDNS(
			ctx,
			domainName,
			createdID,
			marker,
			description,
		)
		resp.Diagnostics.AddError(
			"Unable to verify Dynamic DNS domain",
			dynamicDNSMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyDynamicDNSToResourceModel(ctx, &plan, *domain)...)
	if resp.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreatedDynamicDNS(
			ctx,
			domainName,
			createdID,
			marker,
			description,
		)
		resp.Diagnostics.AddError(
			"Unable to decode Dynamic DNS domain",
			dynamicDNSMutationErrorDetail(
				fmt.Errorf("convert Dynamic DNS metadata"),
				rollbackErr,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dynamicDNSResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DynamicDNSResourceModel
	var state DynamicDNSResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := state.Domain.ValueString()
	current, err := r.client.Get(ctx, domainName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Dynamic DNS domain", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain no longer exists",
			"Refresh the Terraform state before updating the Dynamic DNS domain.",
		)
		return
	}
	expectedID := state.WebcallID.ValueString()
	if expectedID == "" || current.ID != expectedID {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain changed outside Terraform",
			fmt.Sprintf(
				"Refusing to update Dynamic DNS domain %q because its webcall identity no longer matches the Terraform state.",
				domainName,
			),
		)
		return
	}
	previousDescription := state.Description.ValueString()
	if current.Description != previousDescription {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain changed outside Terraform",
			fmt.Sprintf(
				"Refusing to update Dynamic DNS domain %q because its description no longer matches the Terraform state.",
				domainName,
			),
		)
		return
	}

	newDescription := plan.Description.ValueString()
	if newDescription != previousDescription {
		updateErr := r.client.SetDescription(ctx, expectedID, newDescription)
		if updateErr != nil {
			_, applied, reconcileErr := r.reconcileDynamicDNSDescription(
				ctx,
				domainName,
				expectedID,
				previousDescription,
				newDescription,
			)
			if reconcileErr != nil {
				resp.Diagnostics.AddError(
					"Unable to update Dynamic DNS description",
					fmt.Sprintf(
						"%v. Terraform did not attempt a rollback because the remote state could not be attributed safely: %v",
						dynamicDNSCredentialMutationError(updateErr, "description update"),
						reconcileErr,
					),
				)
				return
			}
			if !applied {
				resp.Diagnostics.AddError(
					"Unable to update Dynamic DNS description",
					dynamicDNSCredentialMutationError(
						updateErr,
						"description update",
					).Error(),
				)
				return
			}
		}
	}

	domain, err := r.verifyDynamicDNS(
		ctx,
		domainName,
		newDescription,
		expectedID,
	)
	if err != nil {
		rollbackErr := r.restoreDynamicDNSDescription(
			ctx,
			domainName,
			expectedID,
			newDescription,
			previousDescription,
		)
		resp.Diagnostics.AddError(
			"Unable to verify Dynamic DNS description",
			dynamicDNSMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyDynamicDNSToResourceModel(ctx, &plan, *domain)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dynamicDNSResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DynamicDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domainName := state.Domain.ValueString()
	current, err := r.client.Get(ctx, domainName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Dynamic DNS domain", err.Error())
		return
	}
	if current == nil {
		return
	}
	expectedID := state.WebcallID.ValueString()
	if expectedID == "" || current.ID != expectedID {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain changed outside Terraform",
			fmt.Sprintf(
				"Refusing to delete Dynamic DNS domain %q because its webcall identity no longer matches the Terraform state.",
				domainName,
			),
		)
		return
	}
	expectedDescription := state.Description.ValueString()
	if current.Description != expectedDescription {
		resp.Diagnostics.AddError(
			"Dynamic DNS domain changed outside Terraform",
			fmt.Sprintf(
				"Refusing to delete Dynamic DNS domain %q because its description no longer matches the Terraform state.",
				domainName,
			),
		)
		return
	}

	_, deleteErr := r.client.Delete(ctx, expectedID)
	remaining, readErr := r.client.Get(ctx, domainName)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to verify Dynamic DNS deletion",
			dynamicDNSMutationErrorDetail(
				dynamicDNSCredentialMutationError(deleteErr, "deletion"),
				readErr,
			),
		)
		return
	}
	if remaining == nil {
		return
	}
	if remaining.ID != expectedID {
		resp.Diagnostics.AddWarning(
			"Dynamic DNS domain was replaced during deletion",
			fmt.Sprintf(
				"The managed Dynamic DNS domain %q was deleted, but another domain with the same name now exists. Terraform left the replacement untouched.",
				domainName,
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete Dynamic DNS domain",
			dynamicDNSCredentialMutationError(deleteErr, "deletion").Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to verify Dynamic DNS deletion",
		fmt.Sprintf(
			"cPanel reported success but Dynamic DNS domain %q still exists.",
			domainName,
		),
	)
}

func (r *dynamicDNSResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...,
	)
}

func (r *dynamicDNSResource) Configure(
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

	client, ok := providerData["ddns"].(*ddns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Dynamic DNS Client Type",
			fmt.Sprintf(
				"Expected *ddns.Client, got: %T.",
				providerData["ddns"],
			),
		)
		return
	}

	r.client = client
	r.domainClient = cpaneldomain.NewClient(client.Client)
}

func (r *dynamicDNSResource) verifyDynamicDNS(
	ctx context.Context,
	domainName string,
	expectedDescription string,
	expectedID string,
) (*ddns.Domain, error) {
	domain, err := r.client.Get(ctx, domainName)
	if err != nil {
		return nil, fmt.Errorf("read Dynamic DNS domain after mutation: %w", err)
	}
	if domain == nil {
		return nil, fmt.Errorf(
			"dynamic DNS domain %q was not found after mutation",
			domainName,
		)
	}
	if domain.Description != expectedDescription {
		return nil, fmt.Errorf(
			"dynamic DNS domain %q description is %q; expected %q",
			domainName,
			domain.Description,
			expectedDescription,
		)
	}
	if domain.ID == "" {
		return nil, fmt.Errorf(
			"dynamic DNS domain %q has an empty webcall ID",
			domainName,
		)
	}
	if expectedID != "" && domain.ID != expectedID {
		return nil, fmt.Errorf(
			"dynamic DNS domain %q webcall ID changed unexpectedly",
			domainName,
		)
	}

	return domain, nil
}

func newDynamicDNSCreationMarker() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate Dynamic DNS creation marker: %w", err)
	}

	return "terraform-provider-cpanel:" + hex.EncodeToString(value), nil
}

func (r *dynamicDNSResource) reconcileDynamicDNSDescription(
	ctx context.Context,
	domainName string,
	expectedID string,
	previousDescription string,
	attemptedDescription string,
) (*ddns.Domain, bool, error) {
	current, err := r.client.Get(ctx, domainName)
	if err != nil {
		return nil, false, err
	}
	if current == nil {
		return nil, false, fmt.Errorf(
			"dynamic DNS domain %q no longer exists",
			domainName,
		)
	}
	if current.ID != expectedID {
		return nil, false, fmt.Errorf(
			"dynamic DNS domain %q was replaced",
			domainName,
		)
	}
	switch current.Description {
	case attemptedDescription:
		return current, true, nil
	case previousDescription:
		return current, false, nil
	default:
		return nil, false, fmt.Errorf(
			"dynamic DNS domain %q has a concurrent description",
			domainName,
		)
	}
}

func (r *dynamicDNSResource) restoreDynamicDNSDescription(
	ctx context.Context,
	domainName string,
	expectedID string,
	attemptedDescription string,
	previousDescription string,
) error {
	current, err := r.client.Get(ctx, domainName)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf(
			"cannot restore Dynamic DNS domain %q because it no longer exists",
			domainName,
		)
	}
	if current.ID != expectedID {
		return fmt.Errorf(
			"cannot restore Dynamic DNS domain %q because it was replaced",
			domainName,
		)
	}
	if current.Description == previousDescription {
		return nil
	}
	if current.Description != attemptedDescription {
		return fmt.Errorf(
			"cannot restore Dynamic DNS domain %q because its description changed concurrently",
			domainName,
		)
	}

	restoreErr := r.client.SetDescription(ctx, expectedID, previousDescription)
	reconciled, applied, reconcileErr := r.reconcileDynamicDNSDescription(
		ctx,
		domainName,
		expectedID,
		attemptedDescription,
		previousDescription,
	)
	if reconcileErr != nil {
		if restoreErr != nil {
			return fmt.Errorf(
				"restore Dynamic DNS description: %v; reconcile restoration: %w",
				restoreErr,
				reconcileErr,
			)
		}
		return reconcileErr
	}
	if applied && reconciled != nil {
		return nil
	}
	if restoreErr != nil {
		return restoreErr
	}

	return fmt.Errorf("cPanel reported success but the previous Dynamic DNS description was not restored")
}

func (r *dynamicDNSResource) rollbackCreatedDynamicDNS(
	ctx context.Context,
	domainName string,
	expectedID string,
	allowedDescriptions ...string,
) error {
	current, err := r.client.Get(ctx, domainName)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	if current.ID != expectedID {
		return fmt.Errorf(
			"cannot safely roll back Dynamic DNS creation because domain %q was replaced",
			domainName,
		)
	}
	allowed := false
	for _, description := range allowedDescriptions {
		if current.Description == description {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf(
			"cannot safely roll back Dynamic DNS creation because domain %q changed concurrently",
			domainName,
		)
	}

	_, deleteErr := r.client.Delete(ctx, expectedID)
	remaining, readErr := r.client.Get(ctx, domainName)
	if readErr != nil {
		return fmt.Errorf(
			"%s",
			dynamicDNSMutationErrorDetail(
				dynamicDNSCredentialMutationError(deleteErr, "rollback"),
				readErr,
			),
		)
	}
	if remaining == nil || remaining.ID != expectedID {
		return nil
	}
	if deleteErr != nil {
		return deleteErr
	}

	return fmt.Errorf("cPanel reported success but the created Dynamic DNS domain still exists")
}

func dynamicDNSMutationErrorDetail(primaryError, rollbackError error) string {
	if primaryError == nil {
		if rollbackError == nil {
			return ""
		}

		return rollbackError.Error()
	}
	rollbackError = dynamicDNSCredentialMutationError(
		rollbackError,
		"rollback",
	)
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the Dynamic DNS change: %v",
		primaryError,
		rollbackError,
	)
}

func dynamicDNSCredentialMutationError(err error, operation string) error {
	return sensitiveMutationError(
		err,
		"Dynamic DNS "+operation,
	)
}
