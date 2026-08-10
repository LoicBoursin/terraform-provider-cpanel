package provider

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	cpaneldns "terraform-provider-cpanel/internal/cpanel/dns"
)

var (
	_ resource.Resource                = &dnsRecordResource{}
	_ resource.ResourceWithConfigure   = &dnsRecordResource{}
	_ resource.ResourceWithImportState = &dnsRecordResource{}
)

func NewDNSRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

type dnsRecordResource struct {
	client *cpaneldns.Client
}

func (r *dnsRecordResource) Metadata(
	_ context.Context,
	req resource.MetadataRequest,
	resp *resource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Schema(
	_ context.Context,
	_ resource.SchemaRequest,
	resp *resource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description:         "Manages one DNS record in a cPanel-managed zone.",
		MarkdownDescription: "Manages one DNS record in a cPanel-managed zone.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Required:            true,
				Description:         "The cPanel-managed DNS zone without a trailing dot.",
				MarkdownDescription: "The cPanel-managed DNS zone without a trailing dot.",
				Validators:          domainNameValidators(),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "The record name relative to the zone. Use @ for the zone apex.",
				MarkdownDescription: "The record name relative to the zone. Use `@` for the zone apex.",
				Validators: []validator.String{
					stringvalidator.LengthBetween(1, 253),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"type": schema.StringAttribute{
				Required:            true,
				Description:         "The uppercase DNS record type.",
				MarkdownDescription: "The uppercase DNS record type. Supported values are `A`, `AAAA`, `CAA`, `CNAME`, `MX`, `SRV`, and `TXT`.",
				Validators: []validator.String{
					stringvalidator.OneOf(supportedDNSRecordTypes...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"ttl": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(14400),
				Description:         "The DNS record time to live in seconds.",
				MarkdownDescription: "The DNS record time to live in seconds. Defaults to `14400`.",
				Validators: []validator.Int64{
					int64validator.Between(1, 2147483647),
				},
			},
			"data": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				Description:         "The ordered DNS record data fields. Their number and meaning depend on the record type.",
				MarkdownDescription: "The ordered DNS record data fields. Use `[address]` for `A` and `AAAA`; `[target]` for `CNAME`; `[flag, tag, value]` for `CAA`; `[preference, exchange]` for `MX`; `[priority, weight, port, target]` for `SRV`; and one or more strings for `TXT`.",
				Validators: []validator.List{
					listvalidator.NoNullValues(),
					listvalidator.SizeBetween(1, 16),
					listvalidator.ValueStringsAre(
						stringvalidator.LengthBetween(1, 65535),
					),
				},
			},
			"line_index": schema.Int64Attribute{
				Computed:            true,
				Description:         "The current zero-based line index of the record in the cPanel DNS zone.",
				MarkdownDescription: "The current zero-based line index of the record in the cPanel DNS zone. It may change when other zone records are edited.",
			},
		},
	}
}

func (r *dnsRecordResource) Read(
	ctx context.Context,
	req resource.ReadRequest,
	resp *resource.ReadResponse,
) {
	var state DNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	_, record, err := r.resolveRecord(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS record", err.Error())
		return
	}
	if record == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	if err := validateDNSRecord(
		state.Zone.ValueString(),
		record.Name,
		record.Type,
		record.TTL,
		record.Data,
	); err != nil {
		resp.Diagnostics.AddError(
			"Unsupported DNS record in state",
			fmt.Sprintf("The record at line %d cannot be managed: %v.", record.LineIndex, err),
		)
		return
	}

	resp.Diagnostics.Append(applyDNSRecordToModel(ctx, &state, *record)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Create(
	ctx context.Context,
	req resource.CreateRequest,
	resp *resource.CreateResponse,
) {
	var plan DNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	record, diagnostics := dnsRecordFromModel(ctx, plan)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	zoneName := plan.Zone.ValueString()
	if err := validateDNSRecord(
		zoneName,
		record.Name,
		record.Type,
		record.TTL,
		record.Data,
	); err != nil {
		resp.Diagnostics.AddError("Invalid DNS record", err.Error())
		return
	}

	zone, err := r.client.ParseZone(ctx, zoneName)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS zone", err.Error())
		return
	}
	if len(exactDNSRecordMatches(zone, record)) > 0 {
		resp.Diagnostics.AddError(
			"DNS record already exists",
			"An identical DNS record already exists. Import its zone and line index instead of creating a duplicate.",
		)
		return
	}

	if err := r.client.AddRecord(ctx, zoneName, record); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create DNS record",
			"Could not create DNS record: "+err.Error(),
		)
		return
	}

	createdRecord, err := r.verifyExactRecord(ctx, zoneName, record)
	if err != nil {
		rollbackErr := r.rollbackCreatedRecord(ctx, zoneName, record)
		resp.Diagnostics.AddError(
			"Unable to verify DNS record",
			dnsMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyDNSRecordToModel(ctx, &plan, *createdRecord)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Update(
	ctx context.Context,
	req resource.UpdateRequest,
	resp *resource.UpdateResponse,
) {
	var plan DNSRecordModel
	var state DNSRecordModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDNSRecordManagedState(state); err != nil {
		resp.Diagnostics.AddError("Invalid DNS record state", err.Error())
		return
	}

	desiredRecord, diagnostics := dnsRecordFromModel(ctx, plan)
	resp.Diagnostics.Append(diagnostics...)
	if resp.Diagnostics.HasError() {
		return
	}
	zoneName := plan.Zone.ValueString()
	if err := validateDNSRecord(
		zoneName,
		desiredRecord.Name,
		desiredRecord.Type,
		desiredRecord.TTL,
		desiredRecord.Data,
	); err != nil {
		resp.Diagnostics.AddError("Invalid DNS record", err.Error())
		return
	}

	_, currentRecord, err := r.resolveRecord(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS record", err.Error())
		return
	}
	if currentRecord == nil {
		resp.Diagnostics.AddError(
			"DNS record no longer exists",
			"Refresh the Terraform state before updating the DNS record.",
		)
		return
	}

	lineIndex, err := r.client.UpdateRecord(
		ctx,
		zoneName,
		*currentRecord,
		desiredRecord,
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to update DNS record",
			"Could not update DNS record: "+err.Error(),
		)
		return
	}
	desiredRecord.LineIndex = lineIndex

	updatedRecord, err := r.verifyRecordAtLine(ctx, zoneName, desiredRecord)
	if err != nil {
		rollbackErr := r.rollbackUpdatedRecord(
			ctx,
			zoneName,
			desiredRecord,
			*currentRecord,
		)
		resp.Diagnostics.AddError(
			"Unable to verify DNS record update",
			dnsMutationErrorDetail(err, rollbackErr),
		)
		return
	}

	resp.Diagnostics.Append(applyDNSRecordToModel(ctx, &plan, *updatedRecord)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Delete(
	ctx context.Context,
	req resource.DeleteRequest,
	resp *resource.DeleteResponse,
) {
	var state DNSRecordModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := validateDNSRecordManagedState(state); err != nil {
		resp.Diagnostics.AddError("Invalid DNS record state", err.Error())
		return
	}

	zone, record, err := r.resolveRecord(ctx, state)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read DNS record", err.Error())
		return
	}
	if record == nil {
		return
	}

	matchingCount := len(exactDNSRecordMatches(zone, *record))
	if matchingCount != 1 {
		resp.Diagnostics.AddError(
			"DNS record identity is ambiguous",
			fmt.Sprintf(
				"Refusing to delete DNS record %q because the zone contains %d records with the exact Terraform definition. Refresh and remove duplicates before retrying.",
				record.Name,
				matchingCount,
			),
		)
		return
	}
	deleteErr := r.client.DeleteRecord(
		ctx,
		state.Zone.ValueString(),
		*record,
	)
	refreshedZone, readErr := r.client.ParseZone(
		ctx,
		state.Zone.ValueString(),
	)
	if readErr != nil {
		detail := "Could not verify the DNS record deletion: " + readErr.Error()
		if deleteErr != nil {
			detail = fmt.Sprintf(
				"Could not delete DNS record: %v. %s",
				deleteErr,
				detail,
			)
		}
		resp.Diagnostics.AddError("Unable to verify DNS record deletion", detail)
		return
	}
	remainingCount := len(exactDNSRecordMatches(refreshedZone, *record))
	if remainingCount == 0 {
		return
	}
	if deleteErr != nil {
		resp.Diagnostics.AddError(
			"Unable to delete DNS record",
			"Could not delete DNS record: "+deleteErr.Error(),
		)
		return
	}
	if remainingCount != 0 {
		resp.Diagnostics.AddError(
			"Unable to verify DNS record deletion",
			"cPanel did not remove the unique matching DNS record.",
		)
	}
}

func (r *dnsRecordResource) ImportState(
	ctx context.Context,
	req resource.ImportStateRequest,
	resp *resource.ImportStateResponse,
) {
	separator := strings.LastIndex(req.ID, "/")
	if separator <= 0 || separator == len(req.ID)-1 {
		resp.Diagnostics.AddError(
			"Invalid DNS record import identifier",
			fmt.Sprintf(
				"Expected an identifier in zone/line_index form, got %q.",
				req.ID,
			),
		)
		return
	}

	zone := req.ID[:separator]
	lineIndex, err := strconv.ParseInt(req.ID[separator+1:], 10, 64)
	if err != nil || lineIndex < 0 {
		resp.Diagnostics.AddError(
			"Invalid DNS record import identifier",
			fmt.Sprintf(
				"Expected a non-negative line index in zone/line_index form, got %q.",
				req.ID,
			),
		)
		return
	}
	if err := validateDomainName(zone); err != nil {
		resp.Diagnostics.AddError("Invalid DNS record import zone", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), zone)...)
	resp.Diagnostics.Append(
		resp.State.SetAttribute(ctx, path.Root("line_index"), lineIndex)...,
	)
}

func (r *dnsRecordResource) Configure(
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
			fmt.Sprintf("Expected map[string]interface{}, got: %T.", req.ProviderData),
		)
		return
	}

	client, ok := providerData["dns"].(*cpaneldns.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected DNS Client Type",
			fmt.Sprintf("Expected *dns.Client, got: %T.", providerData["dns"]),
		)
		return
	}

	r.client = client
}

func (r *dnsRecordResource) resolveRecord(
	ctx context.Context,
	state DNSRecordModel,
) (*cpaneldns.Zone, *cpaneldns.Record, error) {
	zone, err := r.client.ParseZone(ctx, state.Zone.ValueString())
	if err != nil {
		return nil, nil, err
	}

	if state.LineIndex.IsNull() || state.LineIndex.IsUnknown() {
		return nil, nil, errors.New("DNS record state does not contain a line index")
	}
	recordAtLine := zone.RecordAt(state.LineIndex.ValueInt64())

	nameKnown := !state.Name.IsNull() && !state.Name.IsUnknown()
	typeKnown := !state.RecordType.IsNull() && !state.RecordType.IsUnknown()
	ttlKnown := !state.TTL.IsNull() && !state.TTL.IsUnknown()
	dataKnown := !state.Data.IsNull() && !state.Data.IsUnknown()
	knownCount := 0
	for _, known := range []bool{nameKnown, typeKnown, ttlKnown, dataKnown} {
		if known {
			knownCount++
		}
	}
	if knownCount == 0 {
		return zone, recordAtLine, nil
	}
	if knownCount != 4 {
		return nil, nil, errors.New(
			"DNS record state contains an incomplete record definition",
		)
	}

	stateData, diagnostics := dnsRecordData(ctx, state.Data)
	if diagnostics.HasError() {
		return nil, nil, errors.New("decode DNS record data from state")
	}

	expected := cpaneldns.Record{
		LineIndex: state.LineIndex.ValueInt64(),
		Name:      state.Name.ValueString(),
		Type:      state.RecordType.ValueString(),
		TTL:       state.TTL.ValueInt64(),
		Data:      stateData,
	}
	record, err := zone.LocateManagedRecord(expected)
	if errors.Is(err, cpaneldns.ErrRecordNotFound) {
		return zone, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf(
			"identify managed %s record %q in zone %q: %w",
			expected.Type,
			expected.Name,
			state.Zone.ValueString(),
			err,
		)
	}

	return zone, record, nil
}

func validateDNSRecordManagedState(state DNSRecordModel) error {
	if state.Zone.IsNull() || state.Zone.IsUnknown() ||
		state.Name.IsNull() || state.Name.IsUnknown() ||
		state.RecordType.IsNull() || state.RecordType.IsUnknown() ||
		state.TTL.IsNull() || state.TTL.IsUnknown() ||
		state.Data.IsNull() || state.Data.IsUnknown() ||
		state.LineIndex.IsNull() || state.LineIndex.IsUnknown() {
		return errors.New(
			"the saved DNS record definition is incomplete; refresh the Terraform state before updating or deleting it",
		)
	}

	return nil
}

func (r *dnsRecordResource) verifyExactRecord(
	ctx context.Context,
	zoneName string,
	expected cpaneldns.Record,
) (*cpaneldns.Record, error) {
	zone, err := r.client.ParseZone(ctx, zoneName)
	if err != nil {
		return nil, err
	}

	matches := exactDNSRecordMatches(zone, expected)
	if len(matches) != 1 {
		return nil, fmt.Errorf(
			"expected exactly one matching DNS record after mutation, found %d",
			len(matches),
		)
	}

	return &matches[0], nil
}

func (r *dnsRecordResource) verifyRecordAtLine(
	ctx context.Context,
	zoneName string,
	expected cpaneldns.Record,
) (*cpaneldns.Record, error) {
	zone, err := r.client.ParseZone(ctx, zoneName)
	if err != nil {
		return nil, err
	}

	record := zone.RecordAt(expected.LineIndex)
	if record == nil || !dnsRecordsEqual(*record, expected) {
		return nil, fmt.Errorf(
			"DNS record at line %d does not match the requested state after mutation",
			expected.LineIndex,
		)
	}

	return record, nil
}

func (r *dnsRecordResource) rollbackCreatedRecord(
	ctx context.Context,
	zoneName string,
	record cpaneldns.Record,
) error {
	zone, err := r.client.ParseZone(ctx, zoneName)
	if err != nil {
		return err
	}
	matches := exactDNSRecordMatches(zone, record)
	switch len(matches) {
	case 0:
		return nil
	case 1:
		return r.client.DeleteRecord(ctx, zoneName, matches[0])
	default:
		return fmt.Errorf(
			"cannot safely roll back creation because %d identical records exist",
			len(matches),
		)
	}
}

func (r *dnsRecordResource) rollbackUpdatedRecord(
	ctx context.Context,
	zoneName string,
	current cpaneldns.Record,
	previous cpaneldns.Record,
) error {
	zone, err := r.client.ParseZone(ctx, zoneName)
	if err != nil {
		return err
	}

	previousMatches := exactDNSRecordMatches(zone, previous)
	if len(previousMatches) == 1 {
		return nil
	}
	if len(previousMatches) > 1 {
		return fmt.Errorf(
			"cannot safely roll back update because %d records match the previous state",
			len(previousMatches),
		)
	}

	currentMatches := exactDNSRecordMatches(zone, current)
	if len(currentMatches) != 1 {
		return fmt.Errorf(
			"cannot safely roll back update because %d records match the attempted state",
			len(currentMatches),
		)
	}

	_, err = r.client.UpdateRecord(ctx, zoneName, currentMatches[0], previous)
	if err == nil {
		return nil
	}

	reconciledZone, readErr := r.client.ParseZone(ctx, zoneName)
	if readErr != nil {
		return fmt.Errorf(
			"restore previous DNS record: %v; read DNS zone after failed restoration: %w",
			err,
			readErr,
		)
	}
	if len(exactDNSRecordMatches(reconciledZone, previous)) == 1 {
		return nil
	}

	return err
}

func exactDNSRecordMatches(
	zone *cpaneldns.Zone,
	expected cpaneldns.Record,
) []cpaneldns.Record {
	var matches []cpaneldns.Record

	for _, record := range zone.Records {
		if dnsRecordsEqual(record, expected) {
			matches = append(matches, record)
		}
	}

	return matches
}

func dnsRecordsEqual(left, right cpaneldns.Record) bool {
	return cpaneldns.RecordsEqual(left, right)
}

func dnsMutationErrorDetail(primaryError, rollbackError error) string {
	if rollbackError == nil {
		return primaryError.Error()
	}

	return fmt.Sprintf(
		"%v. Terraform also failed to roll back the DNS change: %v",
		primaryError,
		rollbackError,
	)
}
