package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-cpanel/internal/cpanel/passenger"
)

type PassengerApplicationResourceModel struct {
	Name                 types.String `tfsdk:"name"`
	Path                 types.String `tfsdk:"path"`
	Domain               types.String `tfsdk:"domain"`
	BaseURI              types.String `tfsdk:"base_uri"`
	DeploymentMode       types.String `tfsdk:"deployment_mode"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	EnvironmentVariables types.Map    `tfsdk:"environment_variables"`
	AbsolutePath         types.String `tfsdk:"absolute_path"`
	DependencyCommands   types.Map    `tfsdk:"dependency_commands"`
	NodeJS               types.String `tfsdk:"nodejs"`
	Python               types.String `tfsdk:"python"`
	Ruby                 types.String `tfsdk:"ruby"`
}

type PassengerApplicationDataSourceModel struct {
	Name                 types.String `tfsdk:"name"`
	Path                 types.String `tfsdk:"path"`
	Domain               types.String `tfsdk:"domain"`
	BaseURI              types.String `tfsdk:"base_uri"`
	DeploymentMode       types.String `tfsdk:"deployment_mode"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	EnvironmentVariables types.Map    `tfsdk:"environment_variables"`
	AbsolutePath         types.String `tfsdk:"absolute_path"`
	DependencyCommands   types.Map    `tfsdk:"dependency_commands"`
	NodeJS               types.String `tfsdk:"nodejs"`
	Python               types.String `tfsdk:"python"`
	Ruby                 types.String `tfsdk:"ruby"`
}

func applyPassengerApplicationToResourceModel(
	ctx context.Context,
	model *PassengerApplicationResourceModel,
	application passenger.Application,
) diag.Diagnostics {
	environmentVariables, diagnostics := types.MapValueFrom(
		ctx,
		types.StringType,
		application.EnvironmentVariables,
	)
	dependencyCommands, dependencyDiagnostics := types.MapValueFrom(
		ctx,
		types.StringType,
		application.DependencyCommands,
	)
	diagnostics.Append(dependencyDiagnostics...)
	if diagnostics.HasError() {
		return diagnostics
	}

	model.Name = types.StringValue(application.Name)
	model.Path = types.StringValue(application.Path)
	model.Domain = types.StringValue(application.Domain)
	model.BaseURI = types.StringValue(application.BaseURI)
	model.DeploymentMode = types.StringValue(application.DeploymentMode)
	model.Enabled = types.BoolValue(application.Enabled)
	model.EnvironmentVariables = environmentVariables
	model.AbsolutePath = types.StringValue(application.AbsolutePath)
	model.DependencyCommands = dependencyCommands
	model.NodeJS = nullableString(application.NodeJS)
	model.Python = nullableString(application.Python)
	model.Ruby = nullableString(application.Ruby)

	return diagnostics
}

func passengerApplicationToDataSourceModel(
	ctx context.Context,
	application passenger.Application,
) (*PassengerApplicationDataSourceModel, diag.Diagnostics) {
	resourceModel := PassengerApplicationResourceModel{}
	diagnostics := applyPassengerApplicationToResourceModel(
		ctx,
		&resourceModel,
		application,
	)
	if diagnostics.HasError() {
		return nil, diagnostics
	}

	return &PassengerApplicationDataSourceModel{
		Name:                 resourceModel.Name,
		Path:                 resourceModel.Path,
		Domain:               resourceModel.Domain,
		BaseURI:              resourceModel.BaseURI,
		DeploymentMode:       resourceModel.DeploymentMode,
		Enabled:              resourceModel.Enabled,
		EnvironmentVariables: resourceModel.EnvironmentVariables,
		AbsolutePath:         resourceModel.AbsolutePath,
		DependencyCommands:   resourceModel.DependencyCommands,
		NodeJS:               resourceModel.NodeJS,
		Python:               resourceModel.Python,
		Ruby:                 resourceModel.Ruby,
	}, diagnostics
}

func passengerApplicationDefinitionFromResourceModel(
	ctx context.Context,
	model PassengerApplicationResourceModel,
) (passenger.Definition, diag.Diagnostics) {
	environmentVariables := map[string]string{}
	var diagnostics diag.Diagnostics
	if !model.EnvironmentVariables.IsNull() &&
		!model.EnvironmentVariables.IsUnknown() {
		diagnostics.Append(
			model.EnvironmentVariables.ElementsAs(
				ctx,
				&environmentVariables,
				false,
			)...,
		)
	}

	return passenger.Definition{
		Name:                 model.Name.ValueString(),
		Path:                 model.Path.ValueString(),
		Domain:               model.Domain.ValueString(),
		BaseURI:              model.BaseURI.ValueString(),
		DeploymentMode:       model.DeploymentMode.ValueString(),
		Enabled:              model.Enabled.ValueBool(),
		EnvironmentVariables: environmentVariables,
	}, diagnostics
}
