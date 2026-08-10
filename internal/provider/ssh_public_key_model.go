package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelssh "terraform-provider-cpanel/internal/cpanel/ssh"
)

type SSHPublicKeyDataSourceModel struct {
	Name              types.String `tfsdk:"name"`
	PublicKey         types.String `tfsdk:"public_key"`
	Authorized        types.Bool   `tfsdk:"authorized"`
	FingerprintSHA256 types.String `tfsdk:"fingerprint_sha256"`
	KeyType           types.String `tfsdk:"key_type"`
	CreatedAt         types.Int64  `tfsdk:"created_at"`
	ModifiedAt        types.Int64  `tfsdk:"modified_at"`
}

type SSHPublicKeysDataSourceModel struct {
	Keys []SSHPublicKeyMetadataModel `tfsdk:"keys"`
}

type SSHPublicKeyMetadataModel struct {
	Name       types.String `tfsdk:"name"`
	Authorized types.Bool   `tfsdk:"authorized"`
	CreatedAt  types.Int64  `tfsdk:"created_at"`
	ModifiedAt types.Int64  `tfsdk:"modified_at"`
}

func sshPublicKeyToDataSourceModel(
	key cpanelssh.PublicKey,
) *SSHPublicKeyDataSourceModel {
	return &SSHPublicKeyDataSourceModel{
		Name:              types.StringValue(key.Name),
		PublicKey:         types.StringValue(key.PublicKey),
		Authorized:        types.BoolValue(key.Authorized),
		FingerprintSHA256: types.StringValue(key.FingerprintSHA256),
		KeyType:           types.StringValue(key.KeyType),
		CreatedAt:         types.Int64Value(key.CreatedAt),
		ModifiedAt:        types.Int64Value(key.ModifiedAt),
	}
}

func sshPublicKeyMetadataToModel(
	key cpanelssh.Metadata,
) SSHPublicKeyMetadataModel {
	return SSHPublicKeyMetadataModel{
		Name:       types.StringValue(key.Name),
		Authorized: types.BoolValue(key.Authorized),
		CreatedAt:  types.Int64Value(key.CreatedAt),
		ModifiedAt: types.Int64Value(key.ModifiedAt),
	}
}
