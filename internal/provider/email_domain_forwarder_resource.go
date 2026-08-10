package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailDomainForwarderResource{}
	_ resource.ResourceWithConfigure   = &emailDomainForwarderResource{}
	_ resource.ResourceWithImportState = &emailDomainForwarderResource{}
)

func NewEmailDomainForwarderResource() resource.Resource {
	return &emailDomainForwarderResource{}
}

type emailDomainForwarderResource struct {
	client *cpanelmail.Client
}

func (r *emailDomainForwarderResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_domain_forwarder"
}

func (r *emailDomainForwarderResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages the domain-level email forwarder for one cPanel mail domain.",
		MarkdownDescription: "Manages the domain-level email forwarder for one cPanel mail domain.",
		Attributes: map[string]schema.Attribute{
			"domain": schema.StringAttribute{
				Required:            true,
				Description:         "The source mail domain owned by the cPanel account.",
				MarkdownDescription: "The source mail domain owned by the cPanel account.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "The external domain that receives mail sent to the source domain.",
				MarkdownDescription: "The external domain that receives mail sent to the source domain.",
				Validators:          domainNameValidators(),
			},
		},
	}
}

func (r *emailDomainForwarderResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailDomainForwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	forwarder, err := r.client.GetDomainForwarder(ctx, state.Domain.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email domain forwarder", err.Error())
		return
	}
	if forwarder == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	state.Destination = types.StringValue(forwarder.Destination)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailDomainForwarderResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailDomainForwarderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := plan.Domain.ValueString()
	destination := plan.Destination.ValueString()
	if err := validateEmailDomain(ctx, r.client, domain); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder domain", err.Error())
		return
	}
	if err := validateDomainName(destination); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder destination", err.Error())
		return
	}

	existing, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email domain forwarder", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Email domain forwarder already exists",
			fmt.Sprintf(
				"Domain %q already forwards to %q. Import it instead of replacing it implicitly.",
				domain,
				existing.Destination,
			),
		)
		return
	}

	if err := r.client.CreateDomainForwarder(ctx, domain, destination); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create email domain forwarder",
			createMutationErrorDetail(
				err,
				"email domain forwarder creation",
				fmt.Sprintf("the email domain forwarder for %q", domain),
			),
		)
		return
	}

	if err := r.verifyDomainForwarder(ctx, domain, destination); err != nil {
		rollbackErr := r.rollbackCreatedDomainForwarder(
			ctx,
			domain,
			destination,
		)
		resp.Diagnostics.AddError(
			"Unable to verify email domain forwarder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailDomainForwarderResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan EmailDomainForwarderModel
	var state EmailDomainForwarderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := plan.Domain.ValueString()
	destination := plan.Destination.ValueString()
	if err := validateEmailDomain(ctx, r.client, domain); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder domain", err.Error())
		return
	}
	if err := validateDomainName(destination); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder destination", err.Error())
		return
	}

	current, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email domain forwarder", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Email domain forwarder no longer exists",
			"Refresh the Terraform state before updating the forwarder.",
		)
		return
	}

	previousDestination := state.Destination.ValueString()
	if current.Destination != previousDestination {
		resp.Diagnostics.AddError(
			"Unable to replace email domain forwarder",
			"The remote email domain forwarder no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteDomainForwarderForReplacement(
		ctx,
		domain,
		previousDestination,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to replace email domain forwarder",
			err.Error(),
		)
		return
	}
	if err := r.client.CreateDomainForwarder(ctx, domain, destination); err != nil {
		rollbackErr := r.restoreDomainForwarder(
			ctx,
			domain,
			destination,
			previousDestination,
		)
		resp.Diagnostics.AddError(
			"Unable to replace email domain forwarder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	if err := r.verifyDomainForwarder(ctx, domain, destination); err != nil {
		rollbackErr := r.restoreDomainForwarder(
			ctx,
			domain,
			destination,
			previousDestination,
		)
		resp.Diagnostics.AddError(
			"Unable to verify email domain forwarder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailDomainForwarderResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailDomainForwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	domain := state.Domain.ValueString()
	existing, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email domain forwarder", err.Error())
		return
	}
	if existing == nil {
		return
	}
	if existing.Destination != state.Destination.ValueString() {
		resp.Diagnostics.AddError(
			"Unable to delete email domain forwarder",
			"The remote email domain forwarder no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteDomainForwarderForReplacement(
		ctx,
		domain,
		existing.Destination,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete email domain forwarder",
			err.Error(),
		)
	}
}

func (r *emailDomainForwarderResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if err := validateDomainName(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid email domain forwarder import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("domain"), req.ID)...,
	)
}

func (r *emailDomainForwarderResource) Configure(
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

	client, ok := providerData["email"].(*cpanelmail.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Email Client Type",
			fmt.Sprintf("Expected *email.Client, got: %T.", providerData["email"]),
		)
		return
	}

	r.client = client
}

func (r *emailDomainForwarderResource) verifyDomainForwarder(
	ctx context.Context,
	domain string,
	destination string,
) error {
	forwarder, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		return fmt.Errorf("read email domain forwarder after mutation: %w", err)
	}
	if forwarder == nil {
		return fmt.Errorf(
			"email domain forwarder for %q was not found after mutation",
			domain,
		)
	}
	if forwarder.Destination != destination {
		return fmt.Errorf(
			"email domain forwarder for %q targets %q, want %q",
			domain,
			forwarder.Destination,
			destination,
		)
	}

	return nil
}

func (r *emailDomainForwarderResource) deleteDomainForwarderForReplacement(
	ctx context.Context,
	domain string,
	originalDestination string,
) error {
	deleteErr := r.client.DeleteDomainForwarder(ctx, domain)
	current, readErr := r.client.GetDomainForwarder(ctx, domain)
	if readErr != nil {
		if deleteErr != nil {
			return fmt.Errorf(
				"delete email domain forwarder: %v; read it after deletion: %w",
				deleteErr,
				readErr,
			)
		}

		return fmt.Errorf("read email domain forwarder after deletion: %w", readErr)
	}
	if current == nil {
		return nil
	}
	if current.Destination != originalDestination {
		return fmt.Errorf(
			"email domain forwarder for %q changed concurrently from destination %q to %q; refusing to continue the replacement",
			domain,
			originalDestination,
			current.Destination,
		)
	}
	if deleteErr != nil {
		return fmt.Errorf(
			"delete email domain forwarder for %q: %w",
			domain,
			deleteErr,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but email domain forwarder for %q still targets %q",
		domain,
		originalDestination,
	)
}

func (r *emailDomainForwarderResource) rollbackCreatedDomainForwarder(
	ctx context.Context,
	domain string,
	attemptedDestination string,
) error {
	current, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		return fmt.Errorf(
			"read created email domain forwarder before rollback: %w",
			err,
		)
	}
	if current == nil {
		return nil
	}
	if current.Destination != attemptedDestination {
		return fmt.Errorf(
			"refuse to roll back email domain forwarder creation because destination changed concurrently from %q to %q",
			attemptedDestination,
			current.Destination,
		)
	}

	return r.deleteDomainForwarderForReplacement(
		ctx,
		domain,
		attemptedDestination,
	)
}

func (r *emailDomainForwarderResource) restoreDomainForwarder(
	ctx context.Context,
	domain string,
	attemptedDestination string,
	originalDestination string,
) error {
	current, err := r.client.GetDomainForwarder(ctx, domain)
	if err != nil {
		return fmt.Errorf(
			"read replacement email domain forwarder before rollback: %w",
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"refuse to restore email domain forwarder for %q because the attempted destination %q is absent",
			domain,
			attemptedDestination,
		)
	}
	if current.Destination == originalDestination {
		return nil
	}
	if current.Destination != attemptedDestination {
		return fmt.Errorf(
			"refuse to restore email domain forwarder for %q because destination changed concurrently from attempted %q to %q",
			domain,
			attemptedDestination,
			current.Destination,
		)
	}

	if err := r.deleteDomainForwarderForReplacement(
		ctx,
		domain,
		attemptedDestination,
	); err != nil {
		return fmt.Errorf("delete replacement email domain forwarder: %w", err)
	}

	createErr := r.client.CreateDomainForwarder(
		ctx,
		domain,
		originalDestination,
	)
	restored, readErr := r.client.GetDomainForwarder(ctx, domain)
	if readErr != nil {
		if createErr != nil {
			return fmt.Errorf(
				"restore previous email domain forwarder: %v; read it after restoration: %w",
				createErr,
				readErr,
			)
		}

		return fmt.Errorf(
			"read email domain forwarder after restoration: %w",
			readErr,
		)
	}
	if restored != nil && restored.Destination == originalDestination {
		return nil
	}
	if restored != nil {
		return fmt.Errorf(
			"email domain forwarder for %q changed concurrently to destination %q while restoring %q",
			domain,
			restored.Destination,
			originalDestination,
		)
	}
	if createErr != nil {
		return fmt.Errorf(
			"restore previous email domain forwarder: %w",
			createErr,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but the previous email domain forwarder for %q was not restored",
		domain,
	)
}
