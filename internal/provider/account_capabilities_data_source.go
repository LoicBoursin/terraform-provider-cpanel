package provider

import (
	"context"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcapabilities "terraform-provider-cpanel/internal/cpanel/capabilities"
)

var (
	_ datasource.DataSource              = &accountCapabilitiesDataSource{}
	_ datasource.DataSourceWithConfigure = &accountCapabilitiesDataSource{}
)

func NewAccountCapabilitiesDataSource() datasource.DataSource {
	return &accountCapabilitiesDataSource{}
}

type accountCapabilitiesDataSource struct {
	client *cpanelcapabilities.Client
}

type AccountCapabilitiesDataSourceModel struct {
	Account          types.String `tfsdk:"account"`
	CPanelVersion    types.String `tfsdk:"cpanel_version"`
	Username         types.String `tfsdk:"username"`
	HomeDirectory    types.String `tfsdk:"home_directory"`
	PrimaryDomain    types.String `tfsdk:"primary_domain"`
	Plan             types.String `tfsdk:"plan"`
	Theme            types.String `tfsdk:"theme"`
	Shell            types.String `tfsdk:"shell"`
	Features         types.Map    `tfsdk:"features"`
	EnabledFeatures  types.Set    `tfsdk:"enabled_features"`
	DisabledFeatures types.Set    `tfsdk:"disabled_features"`
	Limits           types.Map    `tfsdk:"limits"`
}

func (d *accountCapabilitiesDataSource) Metadata(
	_ context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_account_capabilities"
}

func (d *accountCapabilitiesDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Reads cPanel account identity, limits, version, and feature availability.",
		MarkdownDescription: "Reads cPanel account identity, limits, version, and feature availability without exposing contact details, tokens, UUIDs, or other sensitive account fields.",
		Attributes: map[string]schema.Attribute{
			"account": schema.StringAttribute{
				Computed:            true,
				Description:         "The singleton account identity, always account.",
				MarkdownDescription: "The singleton account identity, always `account`.",
			},
			"cpanel_version": schema.StringAttribute{
				Computed:            true,
				Description:         "The complete cPanel version and build string.",
				MarkdownDescription: "The complete cPanel version and build string.",
			},
			"username": schema.StringAttribute{
				Computed:            true,
				Description:         "The cPanel account username.",
				MarkdownDescription: "The cPanel account username.",
			},
			"home_directory": schema.StringAttribute{
				Computed:            true,
				Description:         "The absolute cPanel account home directory.",
				MarkdownDescription: "The absolute cPanel account home directory.",
			},
			"primary_domain": schema.StringAttribute{
				Computed:            true,
				Description:         "The primary domain assigned to the cPanel account.",
				MarkdownDescription: "The primary domain assigned to the cPanel account.",
			},
			"plan": schema.StringAttribute{
				Computed:            true,
				Description:         "The account plan reported by cPanel.",
				MarkdownDescription: "The account plan reported by cPanel.",
			},
			"theme": schema.StringAttribute{
				Computed:            true,
				Description:         "The account theme reported by cPanel.",
				MarkdownDescription: "The account theme reported by cPanel.",
			},
			"shell": schema.StringAttribute{
				Computed:            true,
				Description:         "The account login shell reported by cPanel.",
				MarkdownDescription: "The account login shell reported by cPanel.",
			},
			"features": schema.MapAttribute{
				ElementType:         types.BoolType,
				Computed:            true,
				Description:         "Every cPanel feature name and whether it is enabled.",
				MarkdownDescription: "Every cPanel feature name and whether it is enabled. Use this map for Terraform preconditions on optional cPanel modules.",
			},
			"enabled_features": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The enabled cPanel feature names.",
				MarkdownDescription: "The enabled cPanel feature names.",
			},
			"disabled_features": schema.SetAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "The disabled cPanel feature names.",
				MarkdownDescription: "The disabled cPanel feature names.",
			},
			"limits": schema.MapAttribute{
				ElementType:         types.StringType,
				Computed:            true,
				Description:         "Selected account limits exactly as reported by cPanel.",
				MarkdownDescription: "Selected account limits exactly as reported by cPanel. Values can be numeric strings or `unlimited`.",
			},
		},
	}
}

func (d *accountCapabilitiesDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	capabilities, err := d.client.Get(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read cPanel account capabilities",
			err.Error(),
		)
		return
	}

	features, diagnostics := types.MapValueFrom(
		ctx,
		types.BoolType,
		capabilities.Features,
	)
	resp.Diagnostics.Append(diagnostics...)
	limits, diagnostics := types.MapValueFrom(
		ctx,
		types.StringType,
		capabilities.Limits,
	)
	resp.Diagnostics.Append(diagnostics...)

	enabledFeatureNames := make([]string, 0, len(capabilities.Features))
	disabledFeatureNames := make([]string, 0, len(capabilities.Features))
	for name, enabled := range capabilities.Features {
		if enabled {
			enabledFeatureNames = append(enabledFeatureNames, name)
		} else {
			disabledFeatureNames = append(disabledFeatureNames, name)
		}
	}
	slices.Sort(enabledFeatureNames)
	slices.Sort(disabledFeatureNames)
	enabledFeatures, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		enabledFeatureNames,
	)
	resp.Diagnostics.Append(diagnostics...)
	disabledFeatures, diagnostics := types.SetValueFrom(
		ctx,
		types.StringType,
		disabledFeatureNames,
	)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}

	model := AccountCapabilitiesDataSourceModel{
		Account:          types.StringValue(cpanelcapabilities.AccountIdentity),
		CPanelVersion:    types.StringValue(capabilities.Version),
		Username:         types.StringValue(capabilities.Username),
		HomeDirectory:    types.StringValue(capabilities.HomeDirectory),
		PrimaryDomain:    types.StringValue(capabilities.PrimaryDomain),
		Plan:             types.StringValue(capabilities.Plan),
		Theme:            types.StringValue(capabilities.Theme),
		Shell:            types.StringValue(capabilities.Shell),
		Features:         features,
		EnabledFeatures:  enabledFeatures,
		DisabledFeatures: disabledFeatures,
		Limits:           limits,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (d *accountCapabilitiesDataSource) Configure(
	_ context.Context,
	req datasource.ConfigureRequest,
	resp *datasource.ConfigureResponse,
) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(map[string]interface{})
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				req.ProviderData,
			),
		)
		return
	}

	client, ok := providerData["capabilities"].(*cpanelcapabilities.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Account Capabilities Client Type",
			fmt.Sprintf(
				"Expected *capabilities.Client, got: %T.",
				providerData["capabilities"],
			),
		)
		return
	}

	d.client = client
}
