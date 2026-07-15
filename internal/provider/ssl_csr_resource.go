package provider

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelapi "terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/sslcsr"
)

var (
	_ resource.Resource                = &sslCSRResource{}
	_ resource.ResourceWithConfigure   = &sslCSRResource{}
	_ resource.ResourceWithImportState = &sslCSRResource{}
)

func NewSSLCSRResource() resource.Resource {
	return &sslCSRResource{}
}

type sslCSRClient interface {
	Get(context.Context, string) (*sslcsr.CSR, error)
	Generate(context.Context, sslcsr.Definition) (*sslcsr.CSR, error)
	Rename(
		context.Context,
		sslcsr.Identity,
		string,
		string,
	) (*sslcsr.CSR, error)
	Delete(context.Context, sslcsr.Identity) error
}

type sslCSRResource struct {
	client sslCSRClient
}

func (r *sslCSRResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_ssl_csr"
}

func (r *sslCSRResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	replaceString := []planmodifier.String{
		stringplanmodifier.RequiresReplace(),
	}
	response.Schema = schema.Schema{
		Description:         "Generates and manages one public certificate signing request stored by cPanel. Import requires immutable subject fields that satisfy the resource schema.",
		MarkdownDescription: "Generates and manages one public certificate signing request stored by cPanel from an existing key ID. Terraform reads only the signed public PKCS#10 request and metadata; it never reads, creates, uploads, changes, or deletes private keys. Import requires country, state or province, locality, organization, and every inventory domain in the DNS SAN extension because those immutable values must satisfy the resource schema.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "The CSR identifier assigned by cPanel.",
				MarkdownDescription: "The CSR identifier assigned by cPanel.",
			},
			"key_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Description:         "The existing cPanel SSL key identifier used to generate the CSR.",
				MarkdownDescription: "The existing cPanel SSL key identifier used to generate the CSR. It is required when creating a CSR and is intentionally unavailable after import because cPanel does not expose the originating key ID. Changing it replaces the CSR.",
				Validators:          sslCSRIDValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"friendly_name": schema.StringAttribute{
				Required:            true,
				Description:         "The display name stored by cPanel for the CSR.",
				MarkdownDescription: "The display name stored by cPanel for the CSR. This is the only mutable CSR field.",
				Validators:          sslCSRFriendlyNameValidators(),
			},
			"domains": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The ordered domain list for the CSR.",
				MarkdownDescription: "The ordered domain list for the CSR. The first value becomes the common name and all values become DNS names. Changing the list replaces the CSR.",
				Validators:          sslCSRDomainValidators(),
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"country_name": schema.StringAttribute{
				Required:            true,
				Description:         "The two-letter uppercase country code.",
				MarkdownDescription: "The two-letter uppercase country code. Changing it replaces the CSR.",
				Validators:          sslCSRCountryValidators(),
				PlanModifiers:       replaceString,
			},
			"state_or_province_name": schema.StringAttribute{
				Required:            true,
				Description:         "The certificate subject state or province.",
				MarkdownDescription: "The certificate subject state or province. Changing it replaces the CSR.",
				Validators:          sslCSRRequiredSubjectValidators(),
				PlanModifiers:       replaceString,
			},
			"locality_name": schema.StringAttribute{
				Required:            true,
				Description:         "The certificate subject city or locality.",
				MarkdownDescription: "The certificate subject city or locality. Changing it replaces the CSR.",
				Validators:          sslCSRRequiredSubjectValidators(),
				PlanModifiers:       replaceString,
			},
			"organization_name": schema.StringAttribute{
				Required:            true,
				Description:         "The certificate subject organization.",
				MarkdownDescription: "The certificate subject organization. Changing it replaces the CSR.",
				Validators:          sslCSRRequiredSubjectValidators(),
				PlanModifiers:       replaceString,
			},
			"organizational_unit_name": schema.StringAttribute{
				Optional:            true,
				Description:         "The optional certificate subject organizational unit.",
				MarkdownDescription: "The optional certificate subject organizational unit. Changing it replaces the CSR.",
				Validators:          sslCSROptionalSubjectValidators(),
				PlanModifiers:       replaceString,
			},
			"email_address": schema.StringAttribute{
				Optional:            true,
				Description:         "The optional certificate subject email address.",
				MarkdownDescription: "The optional certificate subject email address. Changing it replaces the CSR.",
				Validators:          sslCSREmailValidators(),
				PlanModifiers:       replaceString,
			},
			"csr": schema.StringAttribute{
				Computed:            true,
				Description:         "The canonical PEM-encoded public PKCS#10 certificate signing request.",
				MarkdownDescription: "The canonical PEM-encoded public PKCS#10 certificate signing request.",
			},
			"fingerprint_sha256": schema.StringAttribute{
				Computed:            true,
				Description:         "The lowercase SHA-256 fingerprint of the PKCS#10 DER bytes.",
				MarkdownDescription: "The lowercase SHA-256 fingerprint of the PKCS#10 DER bytes. Rename and deletion operations require this exact ownership fingerprint.",
			},
			"common_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The CSR common name.",
				MarkdownDescription: "The CSR common name.",
			},
			"created": schema.Int64Attribute{
				Computed:            true,
				Description:         "The CSR creation time reported by cPanel as a Unix timestamp.",
				MarkdownDescription: "The CSR creation time reported by cPanel as a Unix timestamp.",
			},
			"key_algorithm": schema.StringAttribute{
				Computed:            true,
				Description:         "The CSR public-key algorithm reported by cPanel.",
				MarkdownDescription: "The CSR public-key algorithm reported by cPanel.",
			},
			"modulus": schema.StringAttribute{
				Computed:            true,
				Description:         "The RSA public modulus reported by cPanel, when applicable.",
				MarkdownDescription: "The RSA public modulus reported by cPanel, when applicable.",
			},
			"ecdsa_curve_name": schema.StringAttribute{
				Computed:            true,
				Description:         "The ECDSA curve name reported by cPanel, when applicable.",
				MarkdownDescription: "The ECDSA curve name reported by cPanel, when applicable.",
			},
			"ecdsa_public": schema.StringAttribute{
				Computed:            true,
				Description:         "The ECDSA public point reported by cPanel, when applicable.",
				MarkdownDescription: "The ECDSA public point reported by cPanel, when applicable.",
			},
		},
	}
}

func (r *sslCSRResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan SSLCSRResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}
	if plan.KeyID.IsNull() || plan.KeyID.IsUnknown() {
		response.Diagnostics.AddError(
			"Missing SSL key ID",
			"key_id is required when Terraform generates a new cPanel SSL CSR. Imported CSRs can be renamed or deleted without it, but cannot be recreated until a key_id is configured.",
		)
		return
	}

	definition, diagnostics := sslCSRDefinitionFromResourceModel(ctx, plan)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}

	generated, err := r.client.Generate(ctx, definition)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to generate SSL CSR",
			err.Error(),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCSRToResourceModel(ctx, &plan, *generated)...,
	)
	if response.Diagnostics.HasError() {
		response.Diagnostics.AddError(
			"Unable to store SSL CSR state",
			fmt.Sprintf(
				"cPanel SSL CSR %q was generated, but Terraform could not convert its metadata into state. Terraform will not delete it because an ambiguous generation response can be indistinguishable from an equivalent concurrently created CSR. Fix the reported state error, then import this CSR by ID.",
				generated.ID,
			),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		response.Diagnostics.AddError(
			"SSL CSR state was not saved",
			fmt.Sprintf(
				"cPanel SSL CSR %q was generated, but Terraform could not save its state. Terraform will not delete it because an ambiguous generation response can be indistinguishable from an equivalent concurrently created CSR. Fix the reported state error, then import this CSR by ID.",
				generated.ID,
			),
		)
	}
}

func (r *sslCSRResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state SSLCSRResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	identity, err := sslCSRIdentityFromResourceModel(state)
	if err != nil {
		response.Diagnostics.AddError("Invalid SSL CSR state", err.Error())
		return
	}
	current, err := r.client.Get(ctx, identity.ID)
	if err != nil {
		response.Diagnostics.AddError("Unable to read SSL CSR", err.Error())
		return
	}
	if current == nil {
		response.State.RemoveResource(ctx)
		return
	}
	if err := sslcsr.VerifyIdentity(*current, identity); err != nil {
		response.Diagnostics.AddError(
			"SSL CSR identity changed",
			fmt.Sprintf(
				"%v. Terraform refuses to adopt different PKCS#10 material under the same cPanel ID; remove the stale state and import the intended CSR explicitly.",
				err,
			),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCSRToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *sslCSRResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan SSLCSRResourceModel
	var state SSLCSRResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	preserveSSLCSRKeyID(&plan, state)
	identity, err := sslCSRIdentityFromResourceModel(state)
	if err != nil {
		response.Diagnostics.AddError("Invalid SSL CSR state", err.Error())
		return
	}
	previousName := state.FriendlyName.ValueString()
	desiredName := plan.FriendlyName.ValueString()

	updated, err := r.client.Rename(
		ctx,
		identity,
		previousName,
		desiredName,
	)
	if err != nil {
		var rollbackErr error
		if !sslCSRMutationErrorIsDeterministic(err) {
			rollbackErr = r.restoreFriendlyName(
				ctx,
				identity,
				desiredName,
				previousName,
			)
		}
		response.Diagnostics.AddError(
			"Unable to rename SSL CSR",
			sslCSRMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	response.Diagnostics.Append(
		applySSLCSRToResourceModel(ctx, &plan, *updated)...,
	)
	if response.Diagnostics.HasError() {
		rollbackErr := r.restoreFriendlyName(
			ctx,
			identity,
			desiredName,
			previousName,
		)
		response.Diagnostics.AddError(
			"Unable to store SSL CSR state",
			sslCSRMutationErrorDetail(
				fmt.Errorf("convert SSL CSR metadata"),
				rollbackErr,
			),
		)
		return
	}

	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		if rollbackErr := r.restoreFriendlyName(
			ctx,
			identity,
			desiredName,
			previousName,
		); rollbackErr != nil {
			response.Diagnostics.AddError(
				"Unable to roll back SSL CSR rename",
				rollbackErr.Error(),
			)
		}
	}
}

func preserveSSLCSRKeyID(
	plan *SSLCSRResourceModel,
	state SSLCSRResourceModel,
) {
	// cPanel does not expose the originating key ID, so rename operations must
	// preserve the value already owned by state, including null after import.
	plan.KeyID = state.KeyID
}

func (r *sslCSRResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state SSLCSRResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	identity, err := sslCSRIdentityFromResourceModel(state)
	if err != nil {
		response.Diagnostics.AddError("Invalid SSL CSR state", err.Error())
		return
	}
	current, err := r.client.Get(ctx, identity.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read SSL CSR",
			err.Error(),
		)
		return
	}
	if current == nil {
		return
	}
	if current.FingerprintSHA256 != identity.FingerprintSHA256 ||
		current.FriendlyName != state.FriendlyName.ValueString() {
		response.Diagnostics.AddError(
			"Refusing to delete SSL CSR",
			"The stored SSL CSR identity or friendly name no longer matches Terraform state.",
		)
		return
	}

	deleteErr := r.client.Delete(ctx, identity)
	remaining, readErr := r.client.Get(ctx, identity.ID)
	if readErr != nil {
		detail := "Could not verify whether the SSL CSR is absent: " +
			readErr.Error()
		if deleteErr != nil {
			detail = fmt.Sprintf(
				"Could not delete SSL CSR: %v. %s",
				deleteErr,
				detail,
			)
		}
		response.Diagnostics.AddError(
			"Unable to verify SSL CSR deletion",
			detail,
		)
		return
	}
	if remaining == nil {
		return
	}
	if remaining.FingerprintSHA256 != identity.FingerprintSHA256 ||
		remaining.FriendlyName != state.FriendlyName.ValueString() {
		response.Diagnostics.AddWarning(
			"SSL CSR was replaced during deletion",
			fmt.Sprintf(
				"The managed SSL CSR %q was deleted, but another CSR now uses the same identifier. Terraform left the replacement untouched.",
				identity.ID,
			),
		)
		return
	}
	if deleteErr != nil {
		response.Diagnostics.AddError(
			"Unable to delete SSL CSR",
			deleteErr.Error(),
		)
		return
	}

	response.Diagnostics.AddError(
		"Unable to verify SSL CSR deletion",
		fmt.Sprintf(
			"SSL CSR %q still exists after deletion.",
			identity.ID,
		),
	)
}

func (r *sslCSRResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if err := sslcsr.ValidateID(request.ID); err != nil {
		response.Diagnostics.AddError(
			"Invalid SSL CSR import identifier",
			err.Error(),
		)
		return
	}

	current, err := r.client.Get(ctx, request.ID)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import SSL CSR",
			err.Error(),
		)
		return
	}
	if current == nil {
		response.Diagnostics.AddError(
			"SSL CSR not found",
			fmt.Sprintf(
				"SSL CSR %q is not stored in cPanel.",
				request.ID,
			),
		)
		return
	}
	if err := validateSSLCSRResourceRepresentation(*current); err != nil {
		response.Diagnostics.AddError(
			"SSL CSR cannot be imported",
			fmt.Sprintf(
				"SSL CSR %q has immutable metadata that cannot satisfy the cpanel_ssl_csr resource schema: %v. Use the cpanel_ssl_csr data source for read-only access or generate a compatible CSR before importing it.",
				request.ID,
				err,
			),
		)
		return
	}

	state := SSLCSRResourceModel{
		KeyID: types.StringNull(),
	}
	response.Diagnostics.Append(
		applySSLCSRToResourceModel(ctx, &state, *current)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func validateSSLCSRResourceRepresentation(csr sslcsr.CSR) error {
	return sslcsr.ValidateDefinition(sslcsr.Definition{
		// Import validation checks immutable schema values only. This valid
		// placeholder represents the unavailable originating key ID.
		KeyID:                  "import-validation-key",
		FriendlyName:           csr.FriendlyName,
		Domains:                sslCSRDomainsForState(csr),
		CountryName:            csr.CountryName,
		StateOrProvinceName:    csr.StateOrProvinceName,
		LocalityName:           csr.LocalityName,
		OrganizationName:       csr.OrganizationName,
		OrganizationalUnitName: csr.OrganizationalUnitName,
		EmailAddress:           csr.EmailAddress,
	})
}

func (r *sslCSRResource) Configure(
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

	client, ok := providerData["sslcsr"].(*sslcsr.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected SSL CSR Client Type",
			fmt.Sprintf(
				"Expected *sslcsr.Client, got: %T.",
				providerData["sslcsr"],
			),
		)
		return
	}

	r.client = client
}

func (r *sslCSRResource) restoreFriendlyName(
	ctx context.Context,
	identity sslcsr.Identity,
	expectedCurrent string,
	restore string,
) error {
	current, err := r.client.Get(ctx, identity.ID)
	if err != nil {
		return fmt.Errorf(
			"read SSL CSR %q before rename rollback: %w",
			identity.ID,
			err,
		)
	}
	if current == nil {
		return fmt.Errorf(
			"SSL CSR %q no longer exists during rename rollback",
			identity.ID,
		)
	}
	if err := sslcsr.VerifyIdentity(*current, identity); err != nil {
		return err
	}
	if current.FriendlyName == restore {
		return nil
	}
	if current.FriendlyName != expectedCurrent {
		return fmt.Errorf(
			"refuse to restore SSL CSR %q friendly name because it changed concurrently to %q",
			identity.ID,
			current.FriendlyName,
		)
	}
	if _, err := r.client.Rename(
		ctx,
		identity,
		expectedCurrent,
		restore,
	); err != nil {
		return fmt.Errorf(
			"restore SSL CSR %q friendly name: %w",
			identity.ID,
			err,
		)
	}

	return nil
}

func sslCSRMutationErrorIsDeterministic(err error) bool {
	var apiError *cpanelapi.APIError

	return errors.As(err, &apiError)
}

func sslCSRMutationErrorDetail(
	primaryError error,
	rollbackError error,
) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to restore the previous cPanel SSL CSR state: %v",
		primaryError,
		rollbackError,
	)
}
