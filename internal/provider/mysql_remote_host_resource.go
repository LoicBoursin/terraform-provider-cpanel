package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/mysql"
)

var (
	_ resource.Resource                = &mySQLRemoteHostResource{}
	_ resource.ResourceWithConfigure   = &mySQLRemoteHostResource{}
	_ resource.ResourceWithImportState = &mySQLRemoteHostResource{}
)

func NewMySQLRemoteHostResource() resource.Resource {
	return &mySQLRemoteHostResource{}
}

type mySQLRemoteHostResource struct {
	client *mysql.Client
}

func (r *mySQLRemoteHostResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_mysql_remote_host"
}

func (r *mySQLRemoteHostResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Authorizes one remote host to connect to the cPanel account's MySQL or MariaDB databases.",
		MarkdownDescription: "Authorizes one remote host to connect to the cPanel account's MySQL or MariaDB databases.",
		Attributes: map[string]schema.Attribute{
			"host": schema.StringAttribute{
				Required:            true,
				Description:         "The IPv4 address, IPv4 CIDR prefix, IPv4 percent-wildcard pattern, or hostname to authorize.",
				MarkdownDescription: "The IPv4 address, IPv4 CIDR prefix, IPv4 percent-wildcard pattern, or hostname to authorize.",
				Validators:          mySQLRemoteHostValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"note": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(""),
				Description:         "An optional note stored by cPanel for the remote host. Changing it replaces the authorization because cPanel 134 does not reliably update an existing note.",
				MarkdownDescription: "An optional note stored by cPanel for the remote host. Changing it replaces the authorization because cPanel 134 does not reliably update an existing note.",
				Validators:          mySQLRemoteHostNoteValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *mySQLRemoteHostResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state MySQLRemoteHostModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	remoteHost, err := r.client.GetRemoteHost(ctx, state.Host.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read remote MySQL host",
			err.Error(),
		)
		return
	}
	if remoteHost == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	applyMySQLRemoteHostToModel(&state, *remoteHost)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mySQLRemoteHostResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan MySQLRemoteHostModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, err := mysql.NormalizeRemoteHost(plan.Host.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid remote MySQL host", err.Error())
		return
	}
	note := plan.Note.ValueString()
	if err := validateMySQLRemoteHostNote(note); err != nil {
		resp.Diagnostics.AddError("Invalid remote MySQL host note", err.Error())
		return
	}

	existing, err := r.client.GetRemoteHost(ctx, host)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read remote MySQL host",
			err.Error(),
		)
		return
	}
	if existing != nil {
		resp.Diagnostics.AddError(
			"Remote MySQL host already exists",
			fmt.Sprintf(
				"Remote MySQL host %q is already authorized. Import it instead of creating a duplicate.",
				host,
			),
		)
		return
	}

	if err := r.client.AddRemoteHost(ctx, host); err != nil {
		detail := "Could not authorize remote MySQL host: " + err.Error()
		if !cPanelMutationErrorIsDeterministic(err) {
			existing, readErr := r.client.GetRemoteHost(ctx, host)
			switch {
			case readErr != nil:
				detail += ". Terraform could not determine whether the host was authorized; inspect cPanel before retrying."
			case existing != nil:
				detail += ". The host is now authorized, but Terraform did not adopt or remove it because the ambiguous creation cannot be attributed safely. Inspect it and import it if appropriate."
			default:
				detail += ". cPanel did not expose the host after the ambiguous response."
			}
		}
		resp.Diagnostics.AddError(
			"Unable to authorize remote MySQL host",
			detail,
		)
		return
	}
	if note != "" {
		if err := r.client.SetRemoteHostNote(ctx, host, note); err != nil {
			rollbackErr := r.rollbackCreatedRemoteHost(ctx, host, "", note)
			resp.Diagnostics.AddError(
				"Unable to set remote MySQL host note",
				mySQLRemoteHostMutationErrorDetail(err, rollbackErr),
			)
			return
		}
	}

	created, err := r.verifyRemoteHost(ctx, host, note)
	if err != nil {
		rollbackErr := r.rollbackCreatedRemoteHost(ctx, host, "", note)
		resp.Diagnostics.AddError(
			"Unable to verify remote MySQL host",
			mySQLRemoteHostMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	applyMySQLRemoteHostToModel(&plan, *created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mySQLRemoteHostResource) Update(
	_ context.Context,
	_ resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	resp.Diagnostics.AddError(
		"Unsupported remote MySQL host update",
		"Changing the host or note requires replacement.",
	)
}

func (r *mySQLRemoteHostResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state MySQLRemoteHostModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	host, err := mysql.NormalizeRemoteHost(state.Host.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid remote MySQL host in state",
			err.Error(),
		)
		return
	}

	existing, err := r.client.GetRemoteHost(ctx, host)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read remote MySQL host",
			err.Error(),
		)
		return
	}
	if existing == nil {
		return
	}
	expectedNote := state.Note.ValueString()
	if existing.Note != expectedNote {
		resp.Diagnostics.AddError(
			"Remote MySQL host changed outside Terraform",
			fmt.Sprintf(
				"Refusing to delete remote MySQL host %q because its note is now %q instead of %q.",
				host,
				existing.Note,
				expectedNote,
			),
		)
		return
	}

	deleteErr := r.client.DeleteRemoteHost(ctx, host)
	remaining, readErr := r.client.GetRemoteHost(ctx, host)
	if readErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete remote MySQL host",
			mySQLRemoteHostDeleteErrorDetail(deleteErr, readErr),
		)
		return
	}
	if remaining == nil {
		return
	}
	if remaining.Note != expectedNote {
		resp.Diagnostics.AddWarning(
			"Remote MySQL host was replaced during deletion",
			fmt.Sprintf(
				"The managed authorization for %q was deleted, but another authorization with a different note now exists. Terraform left the replacement untouched.",
				host,
			),
		)
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete remote MySQL host",
			"Could not delete remote MySQL host: "+deleteErr.Error(),
		)
		return
	}

	resp.Diagnostics.AddError(
		"Unable to verify remote MySQL host deletion",
		fmt.Sprintf(
			"cPanel reported success but remote MySQL host %q is still authorized.",
			host,
		),
	)
}

func (r *mySQLRemoteHostResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	host, err := mysql.NormalizeRemoteHost(req.ID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid remote MySQL host import ID",
			err.Error(),
		)
		return
	}

	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("host"), host)...,
	)
}

func (r *mySQLRemoteHostResource) Configure(
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
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["mysql"].(*mysql.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected MySQL Client Type",
			fmt.Sprintf(
				"Expected *mysql.Client, got: %T.",
				providerData["mysql"],
			),
		)
		return
	}

	r.client = client
}

func (r *mySQLRemoteHostResource) verifyRemoteHost(
	ctx context.Context,
	host string,
	note string,
) (*mysql.RemoteHost, error) {
	remoteHost, err := r.client.GetRemoteHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf(
			"read remote MySQL hosts after mutation: %w",
			err,
		)
	}
	if remoteHost == nil {
		return nil, fmt.Errorf(
			"remote MySQL host %q was not found after mutation",
			host,
		)
	}
	if remoteHost.Note != note {
		return nil, fmt.Errorf(
			"remote MySQL host %q note is %q; expected %q",
			host,
			remoteHost.Note,
			note,
		)
	}

	return remoteHost, nil
}

func (r *mySQLRemoteHostResource) rollbackCreatedRemoteHost(
	ctx context.Context,
	host string,
	allowedNotes ...string,
) error {
	current, err := r.client.GetRemoteHost(ctx, host)
	if err != nil {
		return err
	}
	if current == nil {
		return nil
	}
	allowed := false
	for _, note := range allowedNotes {
		if current.Note == note {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf(
			"refuse to remove remote MySQL host %q because its note changed concurrently",
			host,
		)
	}

	deleteErr := r.client.DeleteRemoteHost(ctx, host)
	remaining, readErr := r.client.GetRemoteHost(ctx, host)
	if readErr != nil {
		return fmt.Errorf(
			"remove remote MySQL host: %v; verify rollback: %w",
			deleteErr,
			readErr,
		)
	}
	if remaining == nil || remaining.Note != current.Note {
		return nil
	}
	if deleteErr != nil {
		return deleteErr
	}

	return fmt.Errorf(
		"cPanel reported success but remote MySQL host %q still exists after rollback",
		host,
	)
}

func mySQLRemoteHostMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
) string {
	if rollbackErr == nil {
		return mutationErr.Error() + ". The remote host authorization was removed."
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to remove the remote host authorization: %v",
		mutationErr,
		rollbackErr,
	)
}

func mySQLRemoteHostDeleteErrorDetail(deleteErr, readErr error) string {
	if deleteErr == nil {
		return "cPanel reported a successful deletion, but Terraform could not verify that the remote MySQL host is absent: " + readErr.Error()
	}

	return fmt.Sprintf(
		"Could not delete remote MySQL host: %v. Terraform also could not verify whether the authorization still exists: %v",
		deleteErr,
		readErr,
	)
}
