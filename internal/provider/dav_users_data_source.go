package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelcalendar "terraform-provider-cpanel/internal/cpanel/calendar"
)

var (
	_ datasource.DataSource              = &davUsersDataSource{}
	_ datasource.DataSourceWithConfigure = &davUsersDataSource{}
)

func NewDAVUsersDataSource() datasource.DataSource {
	return &davUsersDataSource{}
}

type davUsersDataSource struct {
	client *cpanelcalendar.Client
}

type DAVUsersDataSourceModel struct {
	Users []DAVUserModel `tfsdk:"users"`
}

type DAVUserModel struct {
	Username    types.String         `tfsdk:"username"`
	Collections []DAVCollectionModel `tfsdk:"collections"`
}

type DAVCollectionModel struct {
	Name        types.String `tfsdk:"name"`
	DisplayName types.String `tfsdk:"display_name"`
	Type        types.String `tfsdk:"type"`
}

func (d *davUsersDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_dav_users"
}

func (d *davUsersDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel DAV user and collection inventory.",
		MarkdownDescription: "Reads the complete cPanel DAV user and collection inventory through `CPDAVD::list_users`, with a legacy `CCS` fallback only when cPanel reports that `CPDAVD` is unavailable.",
		Attributes: map[string]schema.Attribute{
			"users": schema.ListNestedAttribute{
				Computed:            true,
				Description:         "The DAV users sorted by complete username.",
				MarkdownDescription: "The DAV users sorted by complete username.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"username": schema.StringAttribute{
							Computed:            true,
							Description:         "The complete DAV username.",
							MarkdownDescription: "The complete DAV username.",
						},
						"collections": schema.ListNestedAttribute{
							Computed:            true,
							Description:         "The user's DAV collections sorted by collection name.",
							MarkdownDescription: "The user's DAV collections sorted by collection name.",
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name": schema.StringAttribute{
										Computed:            true,
										Description:         "The DAV collection identifier.",
										MarkdownDescription: "The DAV collection identifier.",
									},
									"display_name": schema.StringAttribute{
										Computed:            true,
										Description:         "The collection display name, or an empty string when unavailable.",
										MarkdownDescription: "The collection display name, or an empty string when unavailable.",
									},
									"type": schema.StringAttribute{
										Computed:            true,
										Description:         "The DAV collection type reported by cPanel.",
										MarkdownDescription: "The DAV collection type reported by cPanel, such as `VCALENDAR`.",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *davUsersDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	users, err := d.client.ListUsers(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel DAV user inventory",
			err.Error(),
		)
		return
	}

	model := DAVUsersDataSourceModel{
		Users: make([]DAVUserModel, 0, len(users)),
	}
	for _, user := range users {
		userModel := DAVUserModel{
			Username: types.StringValue(user.Username),
			Collections: make(
				[]DAVCollectionModel,
				0,
				len(user.Collections),
			),
		}
		for _, collection := range user.Collections {
			userModel.Collections = append(
				userModel.Collections,
				DAVCollectionModel{
					Name:        types.StringValue(collection.Name),
					DisplayName: types.StringValue(collection.DisplayName),
					Type:        types.StringValue(collection.Type),
				},
			)
		}
		model.Users = append(model.Users, userModel)
	}
	response.Diagnostics.Append(response.State.Set(ctx, &model)...)
}

func (d *davUsersDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	if request.ProviderData == nil {
		return
	}
	providerData, ok := request.ProviderData.(map[string]interface{})
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf(
				"Expected map[string]interface{}, got: %T.",
				request.ProviderData,
			),
		)
		return
	}
	client, ok := providerData["calendar"].(*cpanelcalendar.Client)
	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Calendar Client Type",
			fmt.Sprintf(
				"Expected *calendar.Client, got: %T.",
				providerData["calendar"],
			),
		)
		return
	}
	d.client = client
}
