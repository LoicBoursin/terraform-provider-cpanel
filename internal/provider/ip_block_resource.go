package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/ipblock"
)

var (
	_ resource.Resource                = &ipBlockResource{}
	_ resource.ResourceWithConfigure   = &ipBlockResource{}
	_ resource.ResourceWithImportState = &ipBlockResource{}
)

func NewIPBlockResource() resource.Resource {
	return &ipBlockResource{}
}

type ipBlockResource struct {
	client *ipblock.Client
}

func (r *ipBlockResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_ip_block"
}

func (r *ipBlockResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Blocks one IP address, CIDR prefix, or address range from cPanel-hosted websites.",
		MarkdownDescription: "Blocks one IP address, CIDR prefix, or address range from cPanel-hosted websites.",
		Attributes: map[string]schema.Attribute{
			"address": schema.StringAttribute{
				Required:            true,
				Description:         "The IP address, CIDR prefix, or start-end range to block.",
				MarkdownDescription: "The IP address, CIDR prefix, or `start-end` range to block.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(2, 255),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"start_address": schema.StringAttribute{
				Computed:            true,
				Description:         "The normalized first address in the blocked range.",
				MarkdownDescription: "The normalized first address in the blocked range.",
			},
			"end_address": schema.StringAttribute{
				Computed:            true,
				Description:         "The normalized last address in the blocked range.",
				MarkdownDescription: "The normalized last address in the blocked range.",
			},
		},
	}
}

func (r *ipBlockResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state IPBlockModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	blockedAddress, err := r.client.GetAddress(ctx, state.Address.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to read IP block", err.Error())
		return
	}
	if blockedAddress == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	if err := applyIPBlockToModel(&state, *blockedAddress); err != nil {
		resp.Diagnostics.AddError("Unable to decode IP block", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ipBlockResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan IPBlockModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address, err := ipblock.NormalizeAddress(plan.Address.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid blocked address", err.Error())
		return
	}

	existing, err := r.client.GetAddress(ctx, address)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read IP block", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"IP block already exists",
			"An equivalent IP block already exists. Import it instead of creating a duplicate.",
		)
		return
	}

	if err := r.client.AddAddress(ctx, address); err != nil {
		detail := "Could not create IP block: " + err.Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			existing, readErr := r.client.GetAddress(ctx, address)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the block was created; inspect cPanel before retrying."
			case existing != nil:
				detail += ". An equivalent block now exists, but Terraform did not adopt or remove it because the ambiguous creation cannot be attributed safely. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose an equivalent block after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to create IP block",
			detail,
		)
		return
	}

	created, err := r.client.GetAddress(ctx, address)
	if err != nil || created == nil {
		verificationErr := err
		if verificationErr == nil {
			verificationErr = fmt.Errorf("IP block %q was not found after creation", address)
		}
		resp.Diagnostics.AddError(
			"Unable to verify IP block",
			fmt.Sprintf(
				"%v. Terraform left the remote block untouched because it cannot distinguish the created block from an equivalent concurrent block; inspect cPanel and import it if it exists.",
				verificationErr,
			),
		)
		return
	}

	plan.Address = types.StringValue(address)
	if err := applyIPBlockToModel(&plan, *created); err != nil {
		resp.Diagnostics.AddError(
			"Unable to decode IP block",
			fmt.Sprintf(
				"%v. Terraform left the verified remote block untouched; import it after correcting the state error.",
				err,
			),
		)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ipBlockResource) Update(
	_ context.Context,
	_ resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.Diagnostics.AddError(
		"Unsupported IP block update",
		"Changing a blocked address requires replacement.",
	)
}

func (r *ipBlockResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state IPBlockModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address, err := ipblock.NormalizeAddress(state.Address.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid blocked address in state", err.Error())
		return
	}

	existing, err := r.client.GetAddress(ctx, address)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read IP block", err.Error())
		return
	}
	if existing == nil {
		return
	}

	deleteErr := r.client.RemoveAddress(ctx, address)
	remaining, readErr := r.client.GetAddress(ctx, address)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete IP block",
			ipBlockDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if remaining == nil {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete IP block",
			"Could not delete IP block: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to verify IP block deletion",
		fmt.Sprintf("cPanel reported success but IP block %q still exists.", address),
	)
}

func (r *ipBlockResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	address, err := ipblock.NormalizeAddress(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid IP block import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("address"), address)...,
	)
}

func (r *ipBlockResource) Configure(
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

	client, ok := providerData["ipblock"].(*ipblock.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected IP Block Client Type",
			fmt.Sprintf("Expected *ipblock.Client, got: %T.", providerData["ipblock"]),
		)
		return
	}

	r.client = client
}

func ipBlockDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the IP block is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete IP block: %v. Terraform also could not verify whether the block still exists: %v",
		deleteErr,
		readErr,
	)
}
