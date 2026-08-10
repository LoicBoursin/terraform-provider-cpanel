package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"

	"terraform-provider-cpanel/internal/cpanel/gpg"
)

var (
	_ resource.Resource                = &gpgPublicKeyResource{}
	_ resource.ResourceWithConfigure   = &gpgPublicKeyResource{}
	_ resource.ResourceWithImportState = &gpgPublicKeyResource{}
)

func NewGPGPublicKeyResource() resource.Resource {
	return &gpgPublicKeyResource{}
}

type gpgPublicKeyClient interface {
	Lookup(context.Context, string) (*gpg.Lookup, []string, error)
	Import(context.Context, string) (*gpg.PublicKey, []string, error)
}

type gpgPublicKeyResource struct {
	client gpgPublicKeyClient
}

func (r *gpgPublicKeyResource) Metadata(
	_ context.Context,
	request resource.MetadataRequest,
	response *resource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_gpg_public_key"
}

func (r *gpgPublicKeyResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	response *resource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Imports and tracks one public-only OpenPGP key in cPanel.",
		MarkdownDescription: "Imports and tracks one public-only OpenPGP key in cPanel. Only RSA v4 public keys with 2048, 3072, or 4096 bits are accepted. Terraform never reads, uploads, exports, or deletes private key material. cPanel exposes only a key-pair deletion operation, so removing this resource preserves the remote public key and removes it only from Terraform state.",
		Attributes:          gpgPublicKeyResourceAttributes(),
	}
}

func gpgPublicKeyResourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			Description:         "The uppercase 16-character cPanel GPG public-key identifier.",
			MarkdownDescription: "The uppercase 16-character cPanel GPG public-key identifier.",
		},
		"public_key": schema.StringAttribute{
			Required:            true,
			Description:         "Exactly one ASCII-armored public-only OpenPGP key.",
			MarkdownDescription: "Exactly one ASCII-armored `PGP PUBLIC KEY BLOCK`. The key must contain one RSA v4 entity and no private packets. Armor headers and packet ordering do not participate in identity. Adding, removing, or changing any decoded public packet replaces the resource.",
			Validators:          gpgPublicKeyValidators(),
			PlanModifiers: []planmodifier.String{
				gpgPublicKeySemanticEqualityPlanModifier{},
				stringplanmodifier.RequiresReplace(),
			},
		},
		"fingerprint": schema.StringAttribute{
			Computed:            true,
			Description:         "The full uppercase primary-key fingerprint.",
			MarkdownDescription: "The full uppercase 40-character v4 primary-key fingerprint.",
		},
		"content_sha256": schema.StringAttribute{
			Computed:            true,
			Description:         "The lowercase SHA-256 of the canonical set of decoded public packets.",
			MarkdownDescription: "The lowercase SHA-256 of the canonical set of decoded public packets. Terraform preserves this immutable value and refuses to adopt added, removed, or changed public packets.",
		},
		"algorithm": schema.StringAttribute{
			Computed:            true,
			Description:         "The public-key algorithm text reported by cPanel.",
			MarkdownDescription: "The public-key algorithm text reported by cPanel.",
		},
		"bits": schema.Int64Attribute{
			Computed:            true,
			Description:         "The primary public-key length in bits.",
			MarkdownDescription: "The primary public-key length in bits.",
		},
		"created": schema.Int64Attribute{
			Computed:            true,
			Description:         "The key creation time as a Unix timestamp.",
			MarkdownDescription: "The key creation time as a Unix timestamp.",
		},
		"expires": schema.Int64Attribute{
			Computed:            true,
			Description:         "The key expiration time as a Unix timestamp, when present.",
			MarkdownDescription: "The key expiration time as a Unix timestamp, or null when the key does not expire.",
		},
		"user_id": schema.StringAttribute{
			Computed:            true,
			Description:         "The primary user identity reported by cPanel.",
			MarkdownDescription: "The primary user identity reported by cPanel.",
		},
		"has_secret_key": schema.BoolAttribute{
			Computed:            true,
			Description:         "Whether cPanel reports a matching secret key.",
			MarkdownDescription: "Whether cPanel reports a matching secret key. This resource refuses management whenever the value would be true.",
		},
	}
}

func (r *gpgPublicKeyResource) Create(
	ctx context.Context,
	request resource.CreateRequest,
	response *resource.CreateResponse,
) {
	var plan GPGPublicKeyResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	if response.Diagnostics.HasError() {
		return
	}
	parsed, err := gpg.ParsePublicKey(plan.PublicKey.ValueString())
	if err != nil {
		response.Diagnostics.AddError("Invalid GPG public key", err.Error())
		return
	}

	key, warnings, err := r.client.Import(
		ctx,
		plan.PublicKey.ValueString(),
	)
	addGPGWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to import GPG public key",
			err.Error(),
		)
		return
	}
	if key.ID != parsed.ID ||
		key.Fingerprint != parsed.Fingerprint ||
		key.ContentSHA256 != parsed.ContentSHA256 {
		response.Diagnostics.AddError(
			"Imported GPG public key identity mismatch",
			fmt.Sprintf(
				"cPanel returned a canonical public packet set that does not match the configured key. Terraform preserved the remote key because cPanel exposes only pair deletion. Import key ID %q after resolving the mismatch.",
				key.ID,
			),
		)
		return
	}

	response.Diagnostics.Append(
		applyGPGPublicKeyToResourceModel(&plan, *key, false)...,
	)
	if response.Diagnostics.HasError() {
		r.addCreateStateFailureDiagnostic(&response.Diagnostics, *key)
		return
	}
	stateDiagnostics := response.State.Set(ctx, &plan)
	response.Diagnostics.Append(stateDiagnostics...)
	if stateDiagnostics.HasError() {
		r.addCreateStateFailureDiagnostic(&response.Diagnostics, *key)
	}
}

func (r *gpgPublicKeyResource) Read(
	ctx context.Context,
	request resource.ReadRequest,
	response *resource.ReadResponse,
) {
	var state GPGPublicKeyResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	lookup, warnings, err := r.client.Lookup(ctx, state.ID.ValueString())
	addGPGWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read GPG public key",
			err.Error(),
		)
		return
	}
	if lookup.HasSecretKey {
		response.Diagnostics.AddError(
			"GPG public key is no longer safely manageable",
			fmt.Sprintf(
				"cPanel reports a matching secret key for public key %q. Terraform refuses to manage or delete a key pair.",
				state.ID.ValueString(),
			),
		)
		return
	}
	if lookup.Key == nil {
		response.State.RemoveResource(ctx)
		return
	}
	if err := validateGPGResourceIdentity(state, lookup); err != nil {
		response.Diagnostics.AddError(
			"GPG public key identity changed",
			err.Error(),
		)
		return
	}
	response.Diagnostics.Append(
		applyGPGPublicKeyToResourceModel(
			&state,
			*lookup.Key,
			false,
		)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &state)...)
}

func (r *gpgPublicKeyResource) Update(
	ctx context.Context,
	request resource.UpdateRequest,
	response *resource.UpdateResponse,
) {
	var plan GPGPublicKeyResourceModel
	var state GPGPublicKeyResourceModel
	response.Diagnostics.Append(request.Plan.Get(ctx, &plan)...)
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	lookup, warnings, err := r.client.Lookup(ctx, state.ID.ValueString())
	addGPGWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read GPG public key",
			err.Error(),
		)
		return
	}
	if lookup.Key == nil || lookup.HasSecretKey {
		response.Diagnostics.AddError(
			"GPG public key cannot be updated",
			"The public key is missing or now has matching secret material. Refresh Terraform state before applying.",
		)
		return
	}
	if err := validateGPGResourceIdentity(state, lookup); err != nil {
		response.Diagnostics.AddError(
			"GPG public key identity changed",
			err.Error(),
		)
		return
	}
	response.Diagnostics.Append(
		applyGPGPublicKeyToResourceModel(
			&plan,
			*lookup.Key,
			false,
		)...,
	)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(ctx, &plan)...)
}

func (r *gpgPublicKeyResource) Delete(
	ctx context.Context,
	request resource.DeleteRequest,
	response *resource.DeleteResponse,
) {
	var state GPGPublicKeyResourceModel
	response.Diagnostics.Append(request.State.Get(ctx, &state)...)
	if response.Diagnostics.HasError() {
		return
	}

	response.Diagnostics.AddWarning(
		"GPG public key preserved",
		fmt.Sprintf(
			"cPanel exposes only GPG::delete_keypair, which can delete private material. Terraform therefore removed GPG public key %q from state without deleting it from cPanel.",
			state.ID.ValueString(),
		),
	)
}

func (r *gpgPublicKeyResource) ImportState(
	ctx context.Context,
	request resource.ImportStateRequest,
	response *resource.ImportStateResponse,
) {
	if err := gpg.ValidatePublicID(request.ID); err != nil {
		response.Diagnostics.AddError(
			"Invalid GPG public key import ID",
			err.Error(),
		)
		return
	}
	lookup, warnings, err := r.client.Lookup(ctx, request.ID)
	addGPGWarnings(&response.Diagnostics, warnings)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read GPG public key",
			err.Error(),
		)
		return
	}
	if lookup.Key == nil {
		response.Diagnostics.AddError(
			"GPG public key not found",
			fmt.Sprintf(
				"GPG public key %q is not stored in cPanel.",
				request.ID,
			),
		)
		return
	}
	if lookup.HasSecretKey {
		response.Diagnostics.AddError(
			"Refusing to import GPG key pair",
			fmt.Sprintf(
				"cPanel reports a matching secret key for public key %q. This resource manages public-only keys.",
				request.ID,
			),
		)
		return
	}
	response.Diagnostics.Append(
		response.State.SetAttribute(ctx, path.Root("id"), request.ID)...,
	)
}

func (r *gpgPublicKeyResource) Configure(
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
	client, ok := providerData["gpg"].(*gpg.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected GPG Client Type",
			fmt.Sprintf(
				"Expected *gpg.Client, got: %T.",
				providerData["gpg"],
			),
		)
		return
	}
	r.client = client
}

func (r *gpgPublicKeyResource) addCreateStateFailureDiagnostic(
	diagnostics interface {
		AddError(string, string)
	},
	key gpg.PublicKey,
) {
	diagnostics.AddError(
		"GPG public key state was not saved",
		fmt.Sprintf(
			"cPanel GPG public key %q was imported but Terraform could not save its state. The remote public key was preserved because cPanel exposes only key-pair deletion. Import this ID after resolving the state error.",
			key.ID,
		),
	)
}
