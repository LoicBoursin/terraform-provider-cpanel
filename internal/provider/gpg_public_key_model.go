package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/gpg"
)

type GPGPublicKeyResourceModel struct {
	ID            types.String `tfsdk:"id"`
	PublicKey     types.String `tfsdk:"public_key"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
	Algorithm     types.String `tfsdk:"algorithm"`
	Bits          types.Int64  `tfsdk:"bits"`
	Created       types.Int64  `tfsdk:"created"`
	Expires       types.Int64  `tfsdk:"expires"`
	UserID        types.String `tfsdk:"user_id"`
	HasSecretKey  types.Bool   `tfsdk:"has_secret_key"`
}

type GPGPublicKeyDataSourceModel struct {
	ID            types.String `tfsdk:"id"`
	PublicKey     types.String `tfsdk:"public_key"`
	Fingerprint   types.String `tfsdk:"fingerprint"`
	ContentSHA256 types.String `tfsdk:"content_sha256"`
	Algorithm     types.String `tfsdk:"algorithm"`
	Bits          types.Int64  `tfsdk:"bits"`
	Created       types.Int64  `tfsdk:"created"`
	Expires       types.Int64  `tfsdk:"expires"`
	UserID        types.String `tfsdk:"user_id"`
	HasSecretKey  types.Bool   `tfsdk:"has_secret_key"`
}

func applyGPGPublicKeyToResourceModel(
	model *GPGPublicKeyResourceModel,
	key gpg.PublicKey,
	hasSecretKey bool,
) diag.Diagnostics {
	var diagnostics diag.Diagnostics

	publicKeyValue := types.StringValue(key.Armored)
	if !model.PublicKey.IsNull() && !model.PublicKey.IsUnknown() {
		equal, err := gpg.EqualPublicKey(
			model.PublicKey.ValueString(),
			key.Armored,
		)
		if err != nil {
			diagnostics.AddError(
				"Unable to compare GPG public key state",
				err.Error(),
			)

			return diagnostics
		}
		if equal {
			publicKeyValue = model.PublicKey
		}
	}

	model.ID = types.StringValue(key.ID)
	model.PublicKey = publicKeyValue
	model.Fingerprint = types.StringValue(key.Fingerprint)
	model.ContentSHA256 = types.StringValue(key.ContentSHA256)
	model.Algorithm = types.StringValue(key.Algorithm)
	model.Bits = types.Int64Value(key.Bits)
	model.Created = types.Int64Value(key.Created)
	if key.Expires == nil {
		model.Expires = types.Int64Null()
	} else {
		model.Expires = types.Int64Value(*key.Expires)
	}
	model.UserID = types.StringValue(key.UserID)
	model.HasSecretKey = types.BoolValue(hasSecretKey)

	return diagnostics
}

func gpgPublicKeyToDataSourceModel(
	key gpg.PublicKey,
	hasSecretKey bool,
) (*GPGPublicKeyDataSourceModel, diag.Diagnostics) {
	resourceModel := GPGPublicKeyResourceModel{}
	diagnostics := applyGPGPublicKeyToResourceModel(
		&resourceModel,
		key,
		hasSecretKey,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &GPGPublicKeyDataSourceModel{
		ID:            resourceModel.ID,
		PublicKey:     resourceModel.PublicKey,
		Fingerprint:   resourceModel.Fingerprint,
		ContentSHA256: resourceModel.ContentSHA256,
		Algorithm:     resourceModel.Algorithm,
		Bits:          resourceModel.Bits,
		Created:       resourceModel.Created,
		Expires:       resourceModel.Expires,
		UserID:        resourceModel.UserID,
		HasSecretKey:  resourceModel.HasSecretKey,
	}, diagnostics
}

func validateGPGResourceIdentity(
	model GPGPublicKeyResourceModel,
	lookup *gpg.Lookup,
) error {
	if lookup == nil || lookup.Key == nil {
		return nil
	}
	if !model.Fingerprint.IsNull() &&
		!model.Fingerprint.IsUnknown() &&
		model.Fingerprint.ValueString() != lookup.Key.Fingerprint {
		return fmt.Errorf(
			"cPanel GPG public key %q now has fingerprint %q; Terraform state owns fingerprint %q",
			lookup.Key.ID,
			lookup.Key.Fingerprint,
			model.Fingerprint.ValueString(),
		)
	}
	if !model.ContentSHA256.IsNull() &&
		!model.ContentSHA256.IsUnknown() &&
		model.ContentSHA256.ValueString() != lookup.Key.ContentSHA256 {
		return fmt.Errorf(
			"cPanel GPG public key %q now has public packet SHA-256 %q; Terraform state owns %q",
			lookup.Key.ID,
			lookup.Key.ContentSHA256,
			model.ContentSHA256.ValueString(),
		)
	}

	return nil
}

func addGPGWarnings(
	diagnostics interface {
		AddWarning(string, string)
	},
	warnings []string,
) {
	for _, warning := range warnings {
		diagnostics.AddWarning("cPanel GPG warning", warning)
	}
}
