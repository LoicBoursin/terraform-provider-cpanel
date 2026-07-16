package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

var (
	_ datasource.DataSource              = &emailDomainsDataSource{}
	_ datasource.DataSourceWithConfigure = &emailDomainsDataSource{}
)

func NewEmailDomainsDataSource() datasource.DataSource {
	return &emailDomainsDataSource{}
}

type emailDomainsDataSource struct {
	client *cpanelmail.Client
}

func (d *emailDomainsDataSource) Metadata(
	_ context.Context,
	request datasource.MetadataRequest,
	response *datasource.MetadataResponse,
) {
	response.TypeName = request.ProviderTypeName + "_email_domains"
}

func (d *emailDomainsDataSource) Schema(
	_ context.Context,
	_ datasource.SchemaRequest,
	response *datasource.SchemaResponse,
) {
	response.Schema = schema.Schema{
		Description:         "Reads the complete cPanel mail domain inventory.",
		MarkdownDescription: "Reads the complete normalized cPanel mail domain inventory through `Email::list_mail_domains`.",
		Attributes: map[string]schema.Attribute{
			"domains": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				Description:         "The normalized mail domains in sorted order.",
				MarkdownDescription: "The normalized mail domains in sorted order.",
			},
		},
	}
}

func (d *emailDomainsDataSource) Read(
	ctx context.Context,
	_ datasource.ReadRequest,
	response *datasource.ReadResponse,
) {
	domains, err := d.client.ListMailDomainInventory(ctx)
	if err != nil {
		response.Diagnostics.AddError(
			"Unable to read cPanel mail domain inventory",
			err.Error(),
		)
		return
	}
	state, diagnostics := emailInventoryStringList(ctx, domains)
	response.Diagnostics.Append(diagnostics...)
	if response.Diagnostics.HasError() {
		return
	}
	response.Diagnostics.Append(response.State.Set(
		ctx,
		&EmailDomainInventoryDataSourceModel{Domains: state},
	)...)
}

func (d *emailDomainsDataSource) Configure(
	_ context.Context,
	request datasource.ConfigureRequest,
	response *datasource.ConfigureResponse,
) {
	d.client = configureEmailInventoryClient(request, response)
}
