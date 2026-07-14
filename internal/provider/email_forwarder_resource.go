package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailForwarderResource{}
	_ resource.ResourceWithConfigure   = &emailForwarderResource{}
	_ resource.ResourceWithImportState = &emailForwarderResource{}
)

func NewEmailForwarderResource() resource.Resource {
	return &emailForwarderResource{}
}

type emailForwarderResource struct {
	client *cpanelmail.Client
}

func (r *emailForwarderResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_forwarder"
}

func (r *emailForwarderResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	requiresReplace := []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	}

	resp.Schema = schema.Schema{
		Description:         "Manages one direct email-address forwarder on a cPanel mail domain.",
		MarkdownDescription: "Manages one direct email-address forwarder on a cPanel mail domain.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The complete source address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete source address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
				PlanModifiers:       requiresReplace,
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "The single email address that receives forwarded messages.",
				MarkdownDescription: "The single email address that receives forwarded messages.",
				Validators:          emailAddressValidators(),
				PlanModifiers:       requiresReplace,
			},
		},
	}
}

func (r *emailForwarderResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailForwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, domain, err := splitEmailAccountAddress(state.Address.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder address in state", err.Error())
		return
	}

	forwarder, err := r.client.GetForwarder(
		ctx,
		domain,
		state.Address.ValueString(),
		state.Destination.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email forwarder", err.Error())
		return
	}
	if forwarder == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailForwarderResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailForwarderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := plan.Address.ValueString()
	destination := plan.Destination.ValueString()
	domain, err := validateEmailForwarderSource(ctx, r.client, address)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder address", err.Error())
		return
	}
	if err := validateEmailForwarderDestination(destination); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder destination", err.Error())
		return
	}

	existing, err := r.client.GetForwarder(ctx, domain, address, destination)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email forwarder", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Email forwarder already exists",
			"An identical email forwarder already exists. Import it instead of creating a duplicate.",
		)
		return
	}

	if err := r.client.CreateForwarder(ctx, address, domain, destination); err != nil {
		detail := "Could not create email forwarder: " + err.Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			existing, readErr := r.client.GetForwarder(
				ctx,
				domain,
				address,
				destination,
			)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the forwarder was created; inspect cPanel before retrying."
			case existing != nil:
				detail += ". An identical forwarder now exists, but Terraform did not adopt or delete it because the ambiguous creation cannot be attributed safely. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose an identical forwarder after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create email forwarder",
			detail,
		)
		return
	}

	if err := r.verifyForwarder(ctx, domain, address, destination); err != nil {
		resp.Diagnostics.AddError(
			"Unable to verify email forwarder",
			fmt.Sprintf(
				"%v. Terraform left the remote forwarder untouched because it cannot distinguish the created forwarder from an identical concurrent forwarder; inspect cPanel and import it if it exists.",
				err,
			),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailForwarderResource) Update(
	_ context.Context,
	_ resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.Diagnostics.AddError(
		"Unsupported email forwarder update",
		"Changing an email forwarder address or destination requires replacement.",
	)
}

func (r *emailForwarderResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailForwarderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := state.Address.ValueString()
	destination := state.Destination.ValueString()
	_, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder address in state", err.Error())
		return
	}

	existing, err := r.client.GetForwarder(ctx, domain, address, destination)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email forwarder", err.Error())
		return
	}
	if existing == nil {
		return
	}

	deleteErr := r.client.DeleteForwarder(ctx, address, destination)
	remaining, readErr := r.client.GetForwarder(
		ctx,
		domain,
		address,
		destination,
	)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete email forwarder",
			emailForwarderDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if remaining == nil {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete email forwarder",
			"Could not delete email forwarder: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to verify email forwarder deletion",
		fmt.Sprintf(
			"cPanel reported success but email forwarder %q to %q still exists.",
			address,
			destination,
		),
	)
}

func (r *emailForwarderResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	address, destination, found := strings.Cut(req.ID, "|")
	if !found || address == "" || destination == "" || strings.Contains(destination, "|") {
		resp.Diagnostics.AddError(
			"Invalid email forwarder import identifier",
			fmt.Sprintf(
				"Expected an identifier in address|destination form, got %q.",
				req.ID,
			),
		)
		return
	}
	if _, _, err := splitEmailAccountAddress(address); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder import address", err.Error())
		return
	}
	if err := validateEmailForwarderDestination(destination); err != nil {
		resp.Diagnostics.AddError("Invalid email forwarder import destination", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("address"), address)...)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("destination"), destination)...,
	)
}

func (r *emailForwarderResource) Configure(
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

func (r *emailForwarderResource) verifyForwarder(
	ctx context.Context,
	domain string,
	address string,
	destination string,
) error {
	forwarder, err := r.client.GetForwarder(ctx, domain, address, destination)
	if err != nil {
		return fmt.Errorf("read email forwarder after mutation: %w", err)
	}
	if forwarder == nil {
		return fmt.Errorf(
			"email forwarder %q to %q was not found after mutation",
			address,
			destination,
		)
	}

	return nil
}

func emailForwarderDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the email forwarder is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete email forwarder: %v. Terraform also could not verify whether the forwarder still exists: %v",
		deleteErr,
		readErr,
	)
}
