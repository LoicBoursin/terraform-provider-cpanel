package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ resource.Resource                = &emailAutoResponderResource{}
	_ resource.ResourceWithConfigure   = &emailAutoResponderResource{}
	_ resource.ResourceWithImportState = &emailAutoResponderResource{}
)

func NewEmailAutoResponderResource() resource.Resource {
	return &emailAutoResponderResource{}
}

type emailAutoResponderResource struct {
	client *cpanelmail.Client
}

func (r *emailAutoResponderResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_email_auto_responder"
}

func (r *emailAutoResponderResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one cPanel email autoresponder and its schedule.",
		MarkdownDescription: "Manages one cPanel email autoresponder and its schedule.",
		Attributes: map[string]schema.Attribute{
			"email": schema.StringAttribute{
				Required:            true,
				Description:         "The complete responder email address on a mail domain owned by the cPanel account.",
				MarkdownDescription: "The complete responder email address on a mail domain owned by the cPanel account.",
				Validators:          emailAddressValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"from": schema.StringAttribute{
				Required:            true,
				Description:         "The display name used in autoresponder messages.",
				MarkdownDescription: "The display name used in autoresponder messages.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 255),
				},
			},
			"subject": schema.StringAttribute{
				Required:            true,
				Description:         "The autoresponder message subject.",
				MarkdownDescription: "The autoresponder message subject.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 998),
				},
			},
			"body": schema.StringAttribute{
				Required:            true,
				Description:         "The autoresponder message body. cPanel's trailing newline is normalized in state.",
				MarkdownDescription: "The autoresponder message body. cPanel's trailing newline is normalized in state.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 65535),
				},
			},
			"charset": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("UTF-8"),
				Description:         "The message character set.",
				MarkdownDescription: "The message character set. Defaults to `UTF-8`.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						autoResponderCharsetPattern,
						"Charset names may contain only letters, numbers, dots, underscores, and hyphens.",
					),
				},
			},
			"interval_hours": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(24),
				Description:         "Hours to wait before replying again to the same sender. Use 0 to reply to every message.",
				MarkdownDescription: "Hours to wait before replying again to the same sender. Use `0` to reply to every message.",
				Validators: []validator.Int64{
					int64validator.Between(0, 8760),
				},
			},
			"is_html": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether cPanel should add an HTML content type to the message.",
				MarkdownDescription: "Whether cPanel should add an HTML content type to the message.",
			},
			"start_unix": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Description:         "Unix timestamp at which the autoresponder becomes active. Use 0 for immediate activation.",
				MarkdownDescription: "Unix timestamp at which the autoresponder becomes active. Use `0` for immediate activation.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"stop_unix": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(0),
				Description:         "Unix timestamp at which the autoresponder becomes inactive. Use 0 for no automatic stop.",
				MarkdownDescription: "Unix timestamp at which the autoresponder becomes inactive. Use `0` for no automatic stop.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
		},
	}
}

func (r *emailAutoResponderResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state EmailAutoResponderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, domain, err := splitEmailAccountAddress(state.Email.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid autoresponder email in state", err.Error())
		return
	}

	autoResponder, err := r.client.GetAutoResponder(
		ctx,
		state.Email.ValueString(),
		domain,
	)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email autoresponder", err.Error())
		return
	}
	if autoResponder == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyAutoResponderToModel(&state, *autoResponder)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *emailAutoResponderResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan EmailAutoResponderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid autoresponder email", err.Error())
		return
	}
	if err := validateEmailAutoResponder(plan); err != nil {
		resp.Diagnostics.AddError("Invalid email autoresponder", err.Error())
		return
	}

	existing, err := r.client.GetAutoResponder(ctx, plan.Email.ValueString(), domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email autoresponder", err.Error())
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Email autoresponder already exists",
			"An autoresponder already exists for this address. Import it instead of overwriting it implicitly.",
		)
		return
	}

	desired := autoResponderFromModel(plan)
	if err := r.client.SetAutoResponder(ctx, user, domain, desired); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create email autoresponder",
			createMutationErrorDetail(
				err,
				"email autoresponder creation",
				fmt.Sprintf(
					"the email autoresponder for %q",
					desired.Email,
				),
			),
		)
		return
	}

	created, err := r.verifyAutoResponder(ctx, domain, desired)
	if err != nil {
		rollbackErr := r.rollbackCreatedAutoResponder(ctx, domain, desired)
		resp.Diagnostics.AddError(
			"Unable to verify email autoresponder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyAutoResponderToModel(&plan, *created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAutoResponderResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan EmailAutoResponderModel
	var state EmailAutoResponderModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user, domain, err := validateEmailAccountAddress(
		ctx,
		r.client,
		plan.Email.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("Invalid autoresponder email", err.Error())
		return
	}
	if err := validateEmailAutoResponder(plan); err != nil {
		resp.Diagnostics.AddError("Invalid email autoresponder", err.Error())
		return
	}

	current, err := r.client.GetAutoResponder(ctx, plan.Email.ValueString(), domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email autoresponder", err.Error())
		return
	}
	if current == nil {
		resp.Diagnostics.AddError(
			"Email autoresponder no longer exists",
			"Refresh the Terraform state before updating the autoresponder.",
		)
		return
	}

	original := autoResponderFromModel(state)
	if !autoRespondersEqual(*current, original) {
		resp.Diagnostics.AddError(
			"Unable to update email autoresponder",
			"The remote email autoresponder no longer matches Terraform state, so the provider refuses to overwrite it. Refresh and review the drift before retrying.",
		)
		return
	}

	desired := autoResponderFromModel(plan)
	if err := r.client.SetAutoResponder(ctx, user, domain, desired); err != nil {
		rollbackErr := r.restoreAutoResponder(
			ctx,
			user,
			domain,
			desired,
			original,
		)
		resp.Diagnostics.AddError(
			"Unable to update email autoresponder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	updated, err := r.verifyAutoResponder(ctx, domain, desired)
	if err != nil {
		rollbackErr := r.restoreAutoResponder(
			ctx,
			user,
			domain,
			desired,
			original,
		)
		resp.Diagnostics.AddError(
			"Unable to verify email autoresponder",
			emailMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyAutoResponderToModel(&plan, *updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *emailAutoResponderResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state EmailAutoResponderModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	address := state.Email.ValueString()
	_, domain, err := splitEmailAccountAddress(address)
	if err != nil {
		resp.Diagnostics.AddError("Invalid autoresponder email in state", err.Error())
		return
	}

	existing, err := r.client.GetAutoResponder(ctx, address, domain)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read email autoresponder", err.Error())
		return
	}
	if existing == nil {
		return
	}
	expected := autoResponderFromModel(state)
	if !autoRespondersEqual(*existing, expected) {
		resp.Diagnostics.AddError(
			"Unable to delete email autoresponder",
			"The remote email autoresponder no longer matches Terraform state, so the provider refuses to delete it. Refresh and review the drift before retrying.",
		)
		return
	}

	if err := r.deleteAutoResponderIfCurrentMatches(
		ctx,
		domain,
		expected,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete email autoresponder",
			err.Error(),
		)
	}
}

func (r *emailAutoResponderResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	if _, _, err := splitEmailAccountAddress(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid email autoresponder import ID", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("email"), req.ID)...)
}

func (r *emailAutoResponderResource) Configure(
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

func (r *emailAutoResponderResource) verifyAutoResponder(
	ctx context.Context,
	domain string,
	expected cpanelmail.AutoResponder,
) (*cpanelmail.AutoResponder, error) {
	actual, err := r.client.GetAutoResponder(ctx, expected.Email, domain)
	if err != nil {
		return nil, fmt.Errorf("read email autoresponder after mutation: %w", err)
	}
	if actual == nil {
		return nil, fmt.Errorf(
			"email autoresponder for %q was not found after mutation",
			expected.Email,
		)
	}
	if !autoRespondersEqual(*actual, expected) {
		return nil, fmt.Errorf(
			"email autoresponder for %q does not match the requested state",
			expected.Email,
		)
	}

	return actual, nil
}

func (r *emailAutoResponderResource) deleteAutoResponderIfCurrentMatches(
	ctx context.Context,
	domain string,
	expected cpanelmail.AutoResponder,
) error {
	deleteErr := r.client.DeleteAutoResponder(ctx, expected.Email)
	current, readErr := r.client.GetAutoResponder(
		ctx,
		expected.Email,
		domain,
	)
	if readErr != nil {
		if deleteErr != nil {
			return fmt.Errorf(
				"delete email autoresponder: %v; read it after deletion: %w",
				deleteErr,
				readErr,
			)
		}

		return fmt.Errorf("read email autoresponder after deletion: %w", readErr)
	}
	if current == nil {
		return nil
	}
	if !autoRespondersEqual(*current, expected) {
		return fmt.Errorf(
			"email autoresponder for %q changed concurrently while it was being deleted",
			expected.Email,
		)
	}
	if deleteErr != nil {
		return fmt.Errorf(
			"delete email autoresponder for %q: %w",
			expected.Email,
			deleteErr,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but email autoresponder for %q still exists",
		expected.Email,
	)
}

func (r *emailAutoResponderResource) rollbackCreatedAutoResponder(
	ctx context.Context,
	domain string,
	attempted cpanelmail.AutoResponder,
) error {
	current, err := r.client.GetAutoResponder(ctx, attempted.Email, domain)
	if err != nil {
		return fmt.Errorf(
			"read created email autoresponder before rollback: %w",
			err,
		)
	}
	if current == nil {
		return nil
	}
	if !autoRespondersEqual(*current, attempted) {
		return fmt.Errorf(
			"refuse to roll back email autoresponder creation because the current autoresponder no longer matches the attempted state",
		)
	}

	return r.deleteAutoResponderIfCurrentMatches(ctx, domain, attempted)
}

func (r *emailAutoResponderResource) restoreAutoResponder(
	ctx context.Context,
	user string,
	domain string,
	attempted cpanelmail.AutoResponder,
	original cpanelmail.AutoResponder,
) error {
	current, err := r.client.GetAutoResponder(ctx, attempted.Email, domain)
	if err != nil {
		return fmt.Errorf(
			"read email autoresponder before rollback: %w",
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"refuse to restore email autoresponder for %q because the attempted state is absent",
			attempted.Email,
		)
	}
	if autoRespondersEqual(*current, original) {
		return nil
	}
	if !autoRespondersEqual(*current, attempted) {
		return fmt.Errorf(
			"refuse to restore email autoresponder for %q because it changed concurrently",
			attempted.Email,
		)
	}

	restoreErr := r.client.SetAutoResponder(ctx, user, domain, original)
	restored, readErr := r.client.GetAutoResponder(
		ctx,
		original.Email,
		domain,
	)
	if readErr != nil {
		if restoreErr != nil {
			return fmt.Errorf(
				"restore previous email autoresponder: %v; read it after restoration: %w",
				restoreErr,
				readErr,
			)
		}

		return fmt.Errorf("read email autoresponder after restoration: %w", readErr)
	}
	if restored != nil && autoRespondersEqual(*restored, original) {
		return nil
	}
	if restored != nil && !autoRespondersEqual(*restored, attempted) {
		return fmt.Errorf(
			"email autoresponder for %q changed concurrently while restoring the previous state",
			original.Email,
		)
	}
	if restoreErr != nil {
		return fmt.Errorf("restore previous email autoresponder: %w", restoreErr)
	}
	if restored == nil {
		return fmt.Errorf(
			"cPanel reported success but email autoresponder for %q is absent after restoration",
			original.Email,
		)
	}

	return fmt.Errorf(
		"cPanel reported success but the previous email autoresponder for %q was not restored",
		original.Email,
	)
}
