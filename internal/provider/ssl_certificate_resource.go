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

	"terraform-provider-cpanel/internal/cpanel/sslcertificate"
)

var (
	_ resource.Resource                = &sslCertificateResource{}
	_ resource.ResourceWithConfigure   = &sslCertificateResource{}
	_ resource.ResourceWithImportState = &sslCertificateResource{}
)

func NewSSLCertificateResource() resource.Resource {
	return &sslCertificateResource{}
}

type sslCertificateResource struct {
	client *sslcertificate.Client
}

func (r *sslCertificateResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_certificate"
}

func (r *sslCertificateResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Manages one stored cPanel SSL certificate that is not installed on a domain.",
		MarkdownDescription: "Manages one stored cPanel SSL certificate that is not installed on a domain. Terraform uploads only the public certificate and never reads or stores private keys. Import, update, and destroy refuse certificates that cPanel reports as configured or installed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate identifier assigned by cPanel.",
				MarkdownDescription: "The certificate identifier assigned by cPanel.",
			},
			"friendly_name": schema.StringAttribute{
				Required:            true,
				Description:         "The display name stored by cPanel for the certificate.",
				MarkdownDescription: "The display name stored by cPanel for the certificate.",
				Validators:          sslCertificateFriendlyNameValidators(),
			},
			"certificate": schema.StringAttribute{
				Required:            true,
				Description:         "Exactly one PEM-encoded public X.509 certificate.",
				MarkdownDescription: "Exactly one PEM-encoded public X.509 certificate. Private keys and additional PEM blocks are rejected before any cPanel request. Changing the X.509 certificate replaces the resource.",
				Validators:          sslCertificatePEMValidators(),
				PlanModifiers: []planmodifier.String{
					sslCertificatePEMSemanticEqualityPlanModifier{},
					stringplanmodifier.RequiresReplace(),
				},
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 fingerprint of the certificate DER bytes.",
				MarkdownDescription: "The lowercase SHA-256 fingerprint of the certificate DER bytes.",
			},
			"domains": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The certificate domains reported by cPanel.",
				MarkdownDescription: "The certificate domains reported by cPanel.",
			},
			"created": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate creation time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The certificate creation time reported by cPanel as a Unix timestamp.",
			},
			"not_before": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate validity start as a Unix timestamp.",
				MarkdownDescription: "The certificate validity start as a Unix timestamp.",
			},
			"not_after": schema.Int64Attribute{
				Computed:            true,
				Description:         "The certificate validity end as a Unix timestamp.",
				MarkdownDescription: "The certificate validity end as a Unix timestamp.",
			},
			"serial": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate serial reported by cPanel.",
				MarkdownDescription: "The certificate serial reported by cPanel.",
			},
			"signature_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate signature algorithm reported by cPanel.",
				MarkdownDescription: "The certificate signature algorithm reported by cPanel.",
			},
			"key_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate public-key algorithm reported by cPanel.",
				MarkdownDescription: "The certificate public-key algorithm reported by cPanel.",
			},
			"modulus_length": schema.Int64Attribute{
				Computed:            true,
				Description:         "The public-key modulus length reported by cPanel, when applicable.",
				MarkdownDescription: "The public-key modulus length reported by cPanel, when applicable.",
			},
			"is_self_signed": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the certificate as self-signed.",
				MarkdownDescription: "Whether cPanel reports the certificate as self-signed.",
			},
			"issuer_common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate issuer common name.",
				MarkdownDescription: "The certificate issuer common name.",
			},
			"subject_common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The certificate subject common name.",
				MarkdownDescription: "The certificate subject common name.",
			},
			"validation_type": schema.StringAttribute{
				Computed:            true,
				Description:         "The validation type reported by cPanel, when present.",
				MarkdownDescription: "The validation type reported by cPanel, when present.",
			},
			"domain_is_configured": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports a configured domain for the stored certificate.",
				MarkdownDescription: "Whether cPanel reports a configured domain for the stored certificate. This resource refuses management when the value is true.",
			},
			"installed": schema.BoolAttribute{
				Computed:            true,
				Description:         "Whether cPanel reports the certificate on an installed SSL virtual host.",
				MarkdownDescription: "Whether cPanel reports the certificate on an installed SSL virtual host. This resource refuses management when the value is true.",
			},
		},
	}
}

func (r *sslCertificateResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state SSLCertificateResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	certificate, err := r.client.Get(ctx, state.ID.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL certificate",
			err.Error(),
		)
		return
	}
	if certificate == nil {
		response.State.RemoveResource(ctx)
		return
	}

	if err := r.ensureManageable(ctx, *certificate); err != nil {
		response.Diagnostics.AddError(
			"SSL certificate is no longer safely manageable",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCertificateToResourceModel(
			ctx,
			&state,
			*certificate,
			false,
		)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *sslCertificateResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan SSLCertificateResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}

	friendlyName := plan.FriendlyName.ValueString()
	parsed, err := validateSSLCertificateDefinition(
		friendlyName,
		plan.Certificate.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError("Invalid SSL certificate", err.Error())
		return
	}

	baseline, err := r.client.List(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to inventory SSL certificates",
			err.Error(),
		)
		return
	}
	baselineIDs := make(map[string]struct{}, len(baseline))
	for _, certificate := range baseline {
		baselineIDs[certificate.ID] = struct{}{}
	}

	uploaded, err := r.client.Upload(
		ctx,
		parsed.NormalizedPEM,
		friendlyName,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to upload SSL certificate",
			sslCertificateUnidentifiedUploadErrorDetail(err),
		)
		return
	}
	if uploaded == nil || uploaded.ID == "" {
		response.Diagnostics.AddError(
			"Unable to upload SSL certificate",
			sslCertificateUnidentifiedUploadErrorDetail(
				fmt.Errorf(
					"cPanel reported a successful upload without a certificate ID",
				),
			),
		)
		return
	}
	if _, existed := baselineIDs[uploaded.ID]; existed {
		response.Diagnostics.AddError(
			"SSL certificate already exists",
			"cPanel returned an existing certificate ID. Import that certificate instead of taking ownership implicitly.",
		)
		return
	}

	certificate, err := r.verify(
		ctx,
		uploaded.ID,
		friendlyName,
		parsed.NormalizedPEM,
	)
	if err != nil {
		rollbackErr := r.rollbackCreated(
			ctx,
			uploaded.ID,
			friendlyName,
			parsed.NormalizedPEM,
		)
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate",
			sslCertificateMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCertificateToResourceModel(
			ctx,
			&plan,
			*certificate,
			false,
		)...,
	)
	if response.Diagnostics.HasError() {
		rollbackErr := r.rollbackCreated(
			ctx,
			uploaded.ID,
			friendlyName,
			parsed.NormalizedPEM,
		)
		response.Diagnostics.AddError(
			"Unable to store SSL certificate state",
			sslCertificateMutationErrorDetail(
				fmt.Errorf("convert SSL certificate metadata"),
				rollbackErr,
			),
		)
		return
	}
	stateDiagnostics := response.State.Set(ctx, &plan)
	if stateDiagnostics.HasError() {
		rollbackErr := r.rollbackCreated(
			ctx,
			uploaded.ID,
			friendlyName,
			parsed.NormalizedPEM,
		)
		response.Diagnostics.Append(stateDiagnostics...)
		response.Diagnostics.AddError(
			"Unable to store SSL certificate state",
			sslCertificateMutationErrorDetail(
				fmt.Errorf("write Terraform state"),
				rollbackErr,
			),
		)
		return
	}
	response.Diagnostics.Append(stateDiagnostics...)
}

func (r *sslCertificateResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan SSLCertificateResourceModel
	var state SSLCertificateResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	friendlyName := plan.FriendlyName.ValueString()
	parsed, err := validateSSLCertificateDefinition(
		friendlyName,
		plan.Certificate.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError("Invalid SSL certificate", err.Error())
		return
	}
	stateCertificate, err := sslcertificate.ParsePEM(
		state.Certificate.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate state",
			err.Error(),
		)
		return
	}

	certificate, err := r.client.Get(ctx, state.ID.ValueString())
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL certificate",
			err.Error(),
		)
		return
	}
	if certificate == nil {
		response.Diagnostics.AddError(
			"SSL certificate no longer exists",
			"Refresh the Terraform state before updating the SSL certificate.",
		)
		return
	}
	if err := r.ensureManageable(ctx, *certificate); err != nil {
		response.Diagnostics.AddError(
			"SSL certificate is not safely manageable",
			err.Error(),
		)
		return
	}
	if certificate.FriendlyName != state.FriendlyName.ValueString() {
		response.Diagnostics.AddError(
			"SSL certificate changed during update",
			fmt.Sprintf(
				"SSL certificate %q now has friendly name %q instead of %q from Terraform state. Refresh and review the change before retrying.",
				certificate.ID,
				certificate.FriendlyName,
				state.FriendlyName.ValueString(),
			),
		)
		return
	}
	equal, err := sslcertificate.EqualPEM(
		certificate.CertificatePEM,
		stateCertificate.NormalizedPEM,
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to compare SSL certificate",
			err.Error(),
		)
		return
	}
	if !equal {
		response.Diagnostics.AddError(
			"SSL certificate changed during update",
			"The stored certificate no longer matches Terraform state. Refresh before applying the update.",
		)
		return
	}

	nameChanged := certificate.FriendlyName != friendlyName
	if nameChanged {
		if err := r.client.Rename(
			ctx,
			certificate.ID,
			friendlyName,
		); err != nil {
			rollbackErr := r.restoreFriendlyName(
				ctx,
				*certificate,
				friendlyName,
			)
			response.Diagnostics.AddError(
				"Unable to rename SSL certificate",
				sslCertificateRenameErrorDetail(
					err,
					true,
					rollbackErr,
				),
			)
			return
		}
	}

	updated, err := r.verify(
		ctx,
		certificate.ID,
		friendlyName,
		parsed.NormalizedPEM,
	)
	if err != nil {
		var rollbackErr error
		if nameChanged {
			rollbackErr = r.restoreFriendlyName(
				ctx,
				*certificate,
				friendlyName,
			)
		}
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate update",
			sslCertificateRenameErrorDetail(
				err,
				nameChanged,
				rollbackErr,
			),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCertificateToResourceModel(
			ctx,
			&plan,
			*updated,
			false,
		)...,
	)
	if response.Diagnostics.HasError() {
		if nameChanged {
			if rollbackErr := r.restoreFriendlyName(
				ctx,
				*certificate,
				friendlyName,
			); rollbackErr != nil {
				response.Diagnostics.AddError(
					"Unable to restore SSL certificate",
					rollbackErr.Error(),
				)
			}
		}
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &plan)...)
	if response.Diagnostics.HasError() && nameChanged {
		if rollbackErr := r.restoreFriendlyName(
			ctx,
			*certificate,
			friendlyName,
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to restore SSL certificate",
				rollbackErr.Error(),
			)
		}
	}
}

func (r *sslCertificateResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state SSLCertificateResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueString()
	certificate, err := r.client.Get(ctx, id)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL certificate",
			err.Error(),
		)
		return
	}
	if certificate == nil {
		return
	}
	if err := r.ensureManageable(ctx, *certificate); err != nil {
		response.Diagnostics.AddError(
			"Refusing to delete SSL certificate",
			err.Error(),
		)
		return
	}

	equal, err := sslcertificate.EqualPEM(
		certificate.CertificatePEM,
		state.Certificate.ValueString(),
	)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to compare SSL certificate",
			err.Error(),
		)
		return
	}
	if !equal {
		response.Diagnostics.AddError(
			"Refusing to delete SSL certificate",
			"The stored certificate no longer matches Terraform state.",
		)
		return
	}
	if certificate.FriendlyName != state.FriendlyName.ValueString() {
		response.Diagnostics.AddError(
			"Refusing to delete SSL certificate",
			"The stored certificate friendly name no longer matches Terraform state.",
		)
		return
	}

	deleteErr := r.client.Delete(ctx, id)
	remaining, readErr := r.client.Get(ctx, id)
	if readErr != nil {
		detail := "Could not verify whether the SSL certificate is absent: " +
			readErr.Error()
		if deleteErr != nil {
			detail = fmt.Sprintf(
				"Could not delete SSL certificate: %v. %s",
				deleteErr,
				detail,
			)
		}
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate deletion",
			detail,
		)
		return
	}
	if remaining == nil {
		return
	}
	remainingEqual, comparisonErr := sslcertificate.EqualPEM(
		remaining.CertificatePEM,
		state.Certificate.ValueString(),
	)
	if comparisonErr != nil {
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate deletion",
			comparisonErr.Error(),
		)
		return
	}
	if !remainingEqual ||
		remaining.FriendlyName != state.FriendlyName.ValueString() {
		response.Diagnostics.AddWarning(
			"SSL certificate was replaced during deletion",
			fmt.Sprintf(
				"The managed SSL certificate %q was deleted, but another certificate now uses the same identifier. Terraform left the replacement untouched.",
				id,
			),
		)
		return
	}
	if deleteErr != nil {
		response.Diagnostics.AddError(
			"Unable to delete SSL certificate",
			deleteErr.Error(),
		)
		return
	}
	if remaining != nil {
		response.Diagnostics.AddError(
			"Unable to verify SSL certificate deletion",
			fmt.Sprintf(
				"SSL certificate %q still exists after deletion.",
				id,
			),
		)
	}
}

func (r *sslCertificateResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if err := validateSSLCertificateID(request.ID); err != nil {
		response.Diagnostics.AddError(
			"Invalid SSL certificate import ID",
			err.Error(),
		)
		return
	}

	certificate, err := r.client.Get(ctx, request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL certificate",
			err.Error(),
		)
		return
	}
	if certificate == nil {
		response.Diagnostics.AddError(
			"SSL certificate not found",
			fmt.Sprintf(
				"SSL certificate %q is not stored in cPanel.",
				request.ID,
			),
		)
		return
	}
	if err := r.ensureManageable(ctx, *certificate); err != nil {
		response.Diagnostics.AddError(
			"Refusing to import SSL certificate",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		response.State.SetAttribute(ctx, path.Root("id"), request.ID)...,
	)
}

func (r *sslCertificateResource) Configure(
	_ context.Context,
	request resource.ConfigureRequest,
	response *resource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}

	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["sslcertificate"].(*sslcertificate.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SSL Certificate Client Type",
			fmt.Sprintf(
				"Expected *sslcertificate.Client, got: %T.",
				providerData["sslcertificate"],
			),
		)
		return
	}

	r.client = client
}

func (r *sslCertificateResource) verify(
	ctx context.Context,
	id string,
	friendlyName string,
	certificatePEM string,
) (*sslcertificate.Certificate, error) {
	certificate, err := r.client.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf(
			"read SSL certificate after mutation: %w",
			err,
		)
	}
	if certificate == nil {
		return nil, fmt.Errorf(
			"SSL certificate %q was not found after mutation",
			id,
		)
	}
	if err := r.ensureManageable(ctx, *certificate); err != nil {
		return nil, err
	}
	if certificate.FriendlyName != friendlyName {
		return nil, fmt.Errorf(
			"SSL certificate friendly name is %q; expected %q",
			certificate.FriendlyName,
			friendlyName,
		)
	}
	equal, err := sslcertificate.EqualPEM(
		certificate.CertificatePEM,
		certificatePEM,
	)
	if err != nil {
		return nil, err
	}
	if !equal {
		return nil, fmt.Errorf(
			"stored SSL certificate does not match the uploaded certificate",
		)
	}

	return certificate, nil
}

func (r *sslCertificateResource) ensureManageable(
	ctx context.Context,
	certificate sslcertificate.Certificate,
) error {
	if certificate.DomainIsConfigured {
		return fmt.Errorf(
			"SSL certificate %q is configured for a domain and cannot be managed by this resource",
			certificate.ID,
		)
	}

	installed, err := r.client.IsInstalled(ctx, certificate.ID)
	if err != nil {
		return fmt.Errorf(
			"inspect installed SSL virtual hosts: %w",
			err,
		)
	}
	if installed {
		return fmt.Errorf(
			"SSL certificate %q is installed on an SSL virtual host and cannot be managed by this resource",
			certificate.ID,
		)
	}

	return nil
}

func (r *sslCertificateResource) rollbackCreated(
	ctx context.Context,
	id string,
	expectedFriendlyName string,
	expectedCertificatePEM string,
) error {
	certificate, err := r.client.Get(ctx, id)
	if err != nil {
		return err
	}
	if certificate == nil {
		return nil
	}
	if err := r.ensureManageable(ctx, *certificate); err != nil {
		return err
	}
	if certificate.FriendlyName != expectedFriendlyName {
		return fmt.Errorf(
			"SSL certificate %q friendly name changed before rollback",
			id,
		)
	}
	equal, err := sslcertificate.EqualPEM(
		certificate.CertificatePEM,
		expectedCertificatePEM,
	)
	if err != nil {
		return fmt.Errorf(
			"compare SSL certificate before rollback: %w",
			err,
		)
	}
	if !equal {
		return fmt.Errorf(
			"SSL certificate %q changed before rollback",
			id,
		)
	}
	if err := r.client.Delete(ctx, id); err != nil {
		return err
	}

	remaining, err := r.client.Get(ctx, id)
	if err != nil {
		return err
	}
	if remaining != nil {
		return fmt.Errorf(
			"SSL certificate %q still exists after rollback",
			id,
		)
	}

	return nil
}

func (r *sslCertificateResource) restoreFriendlyName(
	ctx context.Context,
	original sslcertificate.Certificate,
	attemptedFriendlyName string,
) error {
	current, err := r.client.Get(ctx, original.ID)
	if err != nil {
		return fmt.Errorf(
			"read SSL certificate before friendly-name restore: %w",
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"SSL certificate %q no longer exists during friendly-name restore",
			original.ID,
		)
	}
	if err := r.ensureManageable(ctx, *current); err != nil {
		return err
	}
	equal, err := sslcertificate.EqualPEM(
		current.CertificatePEM,
		original.CertificatePEM,
	)
	if err != nil {
		return fmt.Errorf(
			"compare SSL certificate before friendly-name restore: %w",
			err,
		)
	}
	if !equal {
		return fmt.Errorf(
			"SSL certificate %q changed during friendly-name restore",
			original.ID,
		)
	}
	if current.FriendlyName == original.FriendlyName {
		return nil
	}
	if current.FriendlyName != attemptedFriendlyName {
		return fmt.Errorf(
			"refuse to restore SSL certificate %q because its current friendly name %q no longer matches the attempted name %q",
			original.ID,
			current.FriendlyName,
			attemptedFriendlyName,
		)
	}
	if current.FriendlyName != original.FriendlyName {
		if err := r.client.Rename(
			ctx,
			original.ID,
			original.FriendlyName,
		); err != nil {
			return fmt.Errorf(
				"restore previous SSL certificate friendly name: %w",
				err,
			)
		}
	}
	if _, err := r.verify(
		ctx,
		original.ID,
		original.FriendlyName,
		original.CertificatePEM,
	); err != nil {
		return fmt.Errorf(
			"verify restored SSL certificate friendly name: %w",
			err,
		)
	}

	return nil
}

func sslCertificateMutationErrorDetail(
	mutationErr error,
	rollbackErr error,
) string {
	if rollbackErr == nil {
		return mutationErr.Error() + ". The newly stored certificate was removed."
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to remove the newly stored certificate: %v",
		mutationErr,
		rollbackErr,
	)
}

func sslCertificateUnidentifiedUploadErrorDetail(uploadErr error) string {
	return uploadErr.Error() +
		". cPanel did not return a trustworthy new certificate ID, so Terraform did not attempt destructive cleanup."
}

func sslCertificateRenameErrorDetail(
	mutationErr error,
	nameChanged bool,
	rollbackErr error,
) string {
	if !nameChanged {
		return mutationErr.Error()
	}
	if rollbackErr == nil {
		return mutationErr.Error() +
			". The previous friendly name was restored."
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous friendly name: %v",
		mutationErr,
		rollbackErr,
	)
}
