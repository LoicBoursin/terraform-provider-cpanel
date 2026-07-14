package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resourceschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	"terraform-provider-cpanel/internal/cpanel/cron"
)

func TestCronJobStateUpgradeConvertsPublishedLineKey(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	fixtureData, err := os.ReadFile(filepath.Join(
		"testdata",
		"v0.1.0",
		"cron_job_state.json",
	))
	if err != nil {
		t.Fatalf("os.ReadFile() error: %v", err)
	}
	var fixture struct {
		SchemaVersion int64 `json:"schema_version"`
		Attributes    struct {
			LineKey     int64  `json:"linekey"`
			Weekday     string `json:"weekday"`
			Minute      string `json:"minute"`
			Hour        string `json:"hour"`
			Day         string `json:"day"`
			Month       string `json:"month"`
			Command     string `json:"command"`
			LastUpdated string `json:"last_updated"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(fixtureData, &fixture); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if fixture.SchemaVersion != 0 {
		t.Fatalf("fixture schema version = %d; want 0", fixture.SchemaVersion)
	}
	publishedSchema := resourceschema.Schema{
		Attributes: map[string]resourceschema.Attribute{
			"command":      resourceschema.StringAttribute{Required: true},
			"minute":       resourceschema.StringAttribute{Required: true},
			"hour":         resourceschema.StringAttribute{Required: true},
			"day":          resourceschema.StringAttribute{Required: true},
			"weekday":      resourceschema.StringAttribute{Required: true},
			"month":        resourceschema.StringAttribute{Required: true},
			"linekey":      resourceschema.Int64Attribute{Computed: true},
			"last_updated": resourceschema.StringAttribute{Computed: true},
		},
	}

	resourceUnderTest := &cronJobResource{}
	schemaResponse := &frameworkresource.SchemaResponse{}
	resourceUnderTest.Schema(
		ctx,
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	upgrader, ok := resourceUnderTest.UpgradeState(ctx)[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatal("cron job state upgrader for version 0 is missing")
	}
	if got, want := upgrader.PriorSchema.Type().TerraformType(ctx),
		publishedSchema.Type().TerraformType(ctx); !got.Equal(want) {
		t.Fatalf("prior schema type = %s; want published type %s", got, want)
	}

	priorState := tfsdk.State{Schema: publishedSchema}
	diagnostics := priorState.Set(ctx, &struct {
		LineKey     types.Int64  `tfsdk:"linekey"`
		Weekday     types.String `tfsdk:"weekday"`
		Minute      types.String `tfsdk:"minute"`
		Hour        types.String `tfsdk:"hour"`
		Day         types.String `tfsdk:"day"`
		Month       types.String `tfsdk:"month"`
		Command     types.String `tfsdk:"command"`
		LastUpdated types.String `tfsdk:"last_updated"`
	}{
		LineKey:     types.Int64Value(fixture.Attributes.LineKey),
		Weekday:     types.StringValue(fixture.Attributes.Weekday),
		Minute:      types.StringValue(fixture.Attributes.Minute),
		Hour:        types.StringValue(fixture.Attributes.Hour),
		Day:         types.StringValue(fixture.Attributes.Day),
		Month:       types.StringValue(fixture.Attributes.Month),
		Command:     types.StringValue(fixture.Attributes.Command),
		LastUpdated: types.StringValue(fixture.Attributes.LastUpdated),
	})
	if diagnostics.HasError() {
		t.Fatalf("prior State.Set() diagnostics: %v", diagnostics)
	}

	response := &frameworkresource.UpgradeStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	upgrader.StateUpgrader(
		ctx,
		frameworkresource.UpgradeStateRequest{State: &priorState},
		response,
	)
	if response.Diagnostics.HasError() {
		t.Fatalf("StateUpgrader() diagnostics: %v", response.Diagnostics)
	}

	var upgraded CronJobModel
	diagnostics = response.State.Get(ctx, &upgraded)
	if diagnostics.HasError() {
		t.Fatalf("upgraded State.Get() diagnostics: %v", diagnostics)
	}
	if got, want := upgraded.LineKey.ValueString(), "42"; got != want {
		t.Fatalf("upgraded linekey = %q; want %q", got, want)
	}
	if got, want := upgraded.Command.ValueString(), fixture.Attributes.Command; got != want {
		t.Fatalf("upgraded command = %q; want %q", got, want)
	}
}

func TestCronJobDeleteProtectsLineKeyIdentityAndReconcilesResponse(
	t *testing.T,
) {
	t.Parallel()

	managed := cron.CronJobDataSourceDataModel{
		CronJobDetailsModel: cron.CronJobDetailsModel{
			Command: "managed-command",
			Minute:  "7",
			Hour:    "3",
			Day:     "15",
			Weekday: "2",
			Month:   "6",
		},
		LineKey:       "managed-line-key",
		Line:          3,
		CommandNumber: 1,
		Type:          "command",
	}
	replacement := managed
	replacement.Command = "replacement-command"

	testCases := []struct {
		name          string
		current       *cron.CronJobDataSourceDataModel
		deleteOutcome string
		wantError     bool
		wantDeletes   int
	}{
		{
			name:        "changed command is refused before deletion",
			current:     &replacement,
			wantError:   true,
			wantDeletes: 0,
		},
		{
			name:          "ambiguous response with confirmed absence succeeds",
			current:       &managed,
			deleteOutcome: "ambiguous_deleted",
			wantDeletes:   1,
		},
		{
			name:          "replacement after deletion is preserved",
			current:       &managed,
			deleteOutcome: "replacement",
			wantError:     true,
			wantDeletes:   1,
		},
		{
			name:          "false success is reported",
			current:       &managed,
			deleteOutcome: "unchanged",
			wantError:     true,
			wantDeletes:   1,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := cloneCronJob(testCase.current)
			deleteCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				function := request.Form.Get("cpanel_jsonapi_func")
				switch function {
				case cron.OperationFetchCron:
					data := []cron.CronJobDataSourceDataModel{}
					if current != nil {
						data = append(data, *current)
					}
					writeCronJobResourceTestJSON(
						t,
						response,
						map[string]any{
							"cpanelresult": map[string]any{
								"event": map[string]any{"result": 1},
								"data":  data,
							},
						},
					)
				case cron.OperationRemoveLine:
					deleteCalls++
					switch testCase.deleteOutcome {
					case "ambiguous_deleted":
						current = nil
						_, _ = response.Write([]byte("{"))
					case "replacement":
						current = cloneCronJob(&replacement)
						writeCronJobResourceTestJSON(
							t,
							response,
							cronDeleteSuccessResponse(),
						)
					case "unchanged":
						writeCronJobResourceTestJSON(
							t,
							response,
							cronDeleteSuccessResponse(),
						)
					default:
						t.Fatalf(
							"unexpected delete outcome %q",
							testCase.deleteOutcome,
						)
					}
				default:
					t.Fatalf(
						"unexpected request: %s %s function=%q",
						request.Method,
						request.URL.Path,
						function,
					)
				}
			}))
			defer server.Close()

			baseClient, err := cpanel.NewClient(
				server.URL,
				"username",
				"api-token",
			)
			if err != nil {
				t.Fatalf("cpanel.NewClient() error: %v", err)
			}
			response := &frameworkresource.DeleteResponse{}
			(&cronJobResource{
				client: cron.NewClient(baseClient),
			}).Delete(
				t.Context(),
				frameworkresource.DeleteRequest{
					State: testCronJobDeleteState(t, managed),
				},
				response,
			)

			if response.Diagnostics.HasError() != testCase.wantError {
				t.Fatalf(
					"Delete() diagnostics = %v, wantError %t",
					response.Diagnostics,
					testCase.wantError,
				)
			}
			if deleteCalls != testCase.wantDeletes {
				t.Fatalf(
					"delete calls = %d, want %d",
					deleteCalls,
					testCase.wantDeletes,
				)
			}
		})
	}
}

func TestCronJobUpdateRefusesRemoteDrift(t *testing.T) {
	t.Parallel()

	state := CronJobModel{
		LineKey: types.StringValue("managed-line-key"),
		Weekday: types.StringValue("2"),
		Minute:  types.StringValue("7"),
		Hour:    types.StringValue("3"),
		Day:     types.StringValue("15"),
		Month:   types.StringValue("6"),
		Command: types.StringValue("managed-command"),
	}
	plan := state
	plan.Command = types.StringValue("planned-command")

	remote := cron.CronJobDataSourceDataModel{
		CronJobDetailsModel: cron.CronJobDetailsModel{
			Command: "concurrent-command",
			Minute:  state.Minute.ValueString(),
			Hour:    state.Hour.ValueString(),
			Day:     state.Day.ValueString(),
			Weekday: state.Weekday.ValueString(),
			Month:   state.Month.ValueString(),
		},
		LineKey: cron.CronLineKey(state.LineKey.ValueString()),
		Line:    1,
		Type:    "command",
	}
	editCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch request.Form.Get("cpanel_jsonapi_func") {
		case cron.OperationFetchCron:
			writeCronJobResourceTestJSON(
				t,
				response,
				map[string]any{
					"cpanelresult": map[string]any{
						"event": map[string]any{"result": 1},
						"data":  []cron.CronJobDataSourceDataModel{remote},
					},
				},
			)
		case cron.OperationEditLine:
			editCalls++
			t.Fatal("cron edit must not be called after remote drift")
		default:
			t.Fatalf(
				"unexpected cron operation %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
	defer server.Close()

	baseClient, err := cpanel.NewClient(
		server.URL,
		"username",
		"api-token",
	)
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}
	resource := &cronJobResource{client: cron.NewClient(baseClient)}
	response := runSingletonUpdate(
		t,
		NewCronJobResource(),
		state,
		plan,
		resource.Update,
	)
	assertUpdateDriftRefused(t, response.Diagnostics)
	if editCalls != 0 {
		t.Fatalf("edit calls = %d, want 0", editCalls)
	}
}

func TestCronJobModelsEqualChecksEveryField(t *testing.T) {
	t.Parallel()

	managed := CronJobModel{
		LineKey: types.StringValue("managed-line-key"),
		Weekday: types.StringValue("2"),
		Minute:  types.StringValue("7"),
		Hour:    types.StringValue("3"),
		Day:     types.StringValue("15"),
		Month:   types.StringValue("6"),
		Command: types.StringValue("managed-command"),
	}
	testCases := map[string]func(*CronJobModel){
		"line key": func(model *CronJobModel) {
			model.LineKey = types.StringValue("replacement-line-key")
		},
		"weekday": func(model *CronJobModel) {
			model.Weekday = types.StringValue("3")
		},
		"minute": func(model *CronJobModel) {
			model.Minute = types.StringValue("8")
		},
		"hour": func(model *CronJobModel) {
			model.Hour = types.StringValue("4")
		},
		"day": func(model *CronJobModel) {
			model.Day = types.StringValue("16")
		},
		"month": func(model *CronJobModel) {
			model.Month = types.StringValue("7")
		},
		"command": func(model *CronJobModel) {
			model.Command = types.StringValue("replacement-command")
		},
	}

	if !cronJobModelsEqual(managed, managed) {
		t.Fatal("cronJobModelsEqual(managed, managed) = false, want true")
	}
	for name, mutate := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			changed := managed
			mutate(&changed)
			if cronJobModelsEqual(managed, changed) {
				t.Fatal("cronJobModelsEqual() = true, want false")
			}
		})
	}
}

func testCronJobDeleteState(
	t *testing.T,
	job cron.CronJobDataSourceDataModel,
) tfsdk.State {
	t.Helper()

	schemaResponse := &frameworkresource.SchemaResponse{}
	NewCronJobResource().Schema(
		t.Context(),
		frameworkresource.SchemaRequest{},
		schemaResponse,
	)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics: %v", schemaResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diagnostics := state.Set(t.Context(), &CronJobModel{
		LineKey: types.StringValue(string(job.LineKey)),
		Weekday: types.StringValue(job.Weekday),
		Minute:  types.StringValue(job.Minute),
		Hour:    types.StringValue(job.Hour),
		Day:     types.StringValue(job.Day),
		Month:   types.StringValue(job.Month),
		Command: types.StringValue(job.Command),
	})
	if diagnostics.HasError() {
		t.Fatalf("State.Set() diagnostics: %v", diagnostics)
	}

	return state
}

func cloneCronJob(
	job *cron.CronJobDataSourceDataModel,
) *cron.CronJobDataSourceDataModel {
	if job == nil {
		return nil
	}
	cloned := *job

	return &cloned
}

func cronDeleteSuccessResponse() map[string]any {
	return map[string]any{
		"cpanelresult": map[string]any{
			"event": map[string]any{"result": 1},
			"data": []map[string]any{
				{"status": 1, "result": 1},
			},
		},
	}
}

func writeCronJobResourceTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()
	response.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func TestAccCronJobResource(t *testing.T) {
	const resourceName = "cpanel_cron_job.test"

	createCommand := testAccCronCommand("resource-create")
	updateCommand := testAccCronCommand("resource-update")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckCronJobsDestroyed(
			createCommand,
			updateCommand,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccCronJobResourceConfig(
					createCommand,
					"7",
					"3",
					"15",
					"2",
					"6",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "command", createCommand),
					resource.TestCheckResourceAttr(resourceName, "minute", "7"),
					resource.TestCheckResourceAttr(resourceName, "hour", "3"),
					resource.TestCheckResourceAttr(resourceName, "day", "15"),
					resource.TestCheckResourceAttr(resourceName, "weekday", "2"),
					resource.TestCheckResourceAttr(resourceName, "month", "6"),
					resource.TestCheckResourceAttrSet(resourceName, "linekey"),
					testAccCheckCronJobExists(createCommand),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportState:                          true,
				ImportStateIdFunc:                    testAccImportStateIDFromAttribute(resourceName, "linekey"),
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "linekey",
			},
			{
				Config: testAccCronJobResourceConfig(
					updateCommand,
					"11",
					"4",
					"20",
					"5",
					"9",
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "command", updateCommand),
					resource.TestCheckResourceAttr(resourceName, "minute", "11"),
					resource.TestCheckResourceAttr(resourceName, "hour", "4"),
					resource.TestCheckResourceAttr(resourceName, "day", "20"),
					resource.TestCheckResourceAttr(resourceName, "weekday", "5"),
					resource.TestCheckResourceAttr(resourceName, "month", "9"),
					resource.TestCheckResourceAttrSet(resourceName, "linekey"),
					testAccCheckCronJobExists(updateCommand),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteCronJobs(t, updateCommand)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccCronJobResourceConfig(
					updateCommand,
					"11",
					"4",
					"20",
					"5",
					"9",
				),
				Check: testAccCheckCronJobExists(updateCommand),
			},
		},
	})
}

func testAccCronJobResourceConfig(command, minute, hour, day, weekday, month string) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_cron_job" "test" {
  command = %q
  minute = %q
  hour = %q
  day = %q
  weekday = %q
  month = %q
}
`, command, minute, hour, day, weekday, month)
}
