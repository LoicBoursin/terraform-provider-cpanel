package cron

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestCronLineKeyUnmarshalJSON(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input     string
		want      CronLineKey
		wantError bool
	}{
		"string": {
			input: `"f32e3d460c179443e5f772359c7954ec"`,
			want:  "f32e3d460c179443e5f772359c7954ec",
		},
		"integer": {
			input: "42",
			want:  "42",
		},
		"empty string": {
			input:     `""`,
			wantError: true,
		},
		"negative integer": {
			input:     "-1",
			wantError: true,
		},
		"decimal": {
			input:     "4.2",
			wantError: true,
		},
		"null": {
			input:     "null",
			wantError: true,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var lineKey CronLineKey
			err := json.Unmarshal([]byte(testCase.input), &lineKey)
			if testCase.wantError && err == nil {
				t.Fatal("UnmarshalJSON() error = nil, want error")
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("UnmarshalJSON() error = %v", err)
			}
			if lineKey != testCase.want {
				t.Fatalf("UnmarshalJSON() = %q, want %q", lineKey, testCase.want)
			}
		})
	}
}

func TestClientReadsCronJobs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertCronAPI2Request(
			t,
			request,
			http.MethodGet,
			OperationFetchCron,
			url.Values{},
		)
		writeCronJSON(
			t,
			response,
			`{"cpanelresult":{"apiversion":2,"func":"fetchcron","module":"Cron","event":{"result":1},"data":[{"linekey":"f32e3d460c179443e5f772359c7954ec","line":2,"command":"/usr/bin/true","minute":"7","hour":"3","day":"15","weekday":"2","month":"6","value":"7 3 15 6 2 /usr/bin/true","type":"command","key":"command","count":"1","commandnumber":1,"command_htmlsafe":"/usr/bin/true","reason":"","result":true}]}}`, // gitleaks:allow
		)
	}))
	defer server.Close()

	jobs, err := newCronTestClient(t, server.URL).GetCronJobs(t.Context())
	if err != nil {
		t.Fatalf("GetCronJobs() error: %v", err)
	}
	if len(jobs.CpanelResult.Data) != 1 {
		t.Fatalf("cron job count = %d, want 1", len(jobs.CpanelResult.Data))
	}
	job := jobs.CpanelResult.Data[0]
	if job.LineKey != "f32e3d460c179443e5f772359c7954ec" ||
		job.Line != 2 ||
		job.Command != "/usr/bin/true" ||
		job.Minute != "7" ||
		job.Hour != "3" ||
		job.Day != "15" ||
		job.Weekday != "2" ||
		job.Month != "6" {
		t.Fatalf("cron job = %#v", job)
	}
}

func TestClientCronMutationsUsePOST(t *testing.T) {
	t.Parallel()

	const command = `/usr/bin/printf 'token=&=?'`

	testCases := []struct {
		name       string
		function   string
		parameters url.Values
		call       func(*Client) error
	}{
		{
			name:     "create",
			function: OperationAddLine,
			parameters: url.Values{
				"command": {command},
				"minute":  {"7"},
				"hour":    {"3"},
				"day":     {"15"},
				"weekday": {"2"},
				"month":   {"6"},
			},
			call: func(client *Client) error {
				created, err := client.CreateCronJob(
					context.Background(),
					CronJobCreateModel{CronJobDetailsModel: CronJobDetailsModel{
						Command: command,
						Minute:  "7",
						Hour:    "3",
						Day:     "15",
						Weekday: "2",
						Month:   "6",
					}},
				)
				if err != nil {
					return err
				}
				if len(created.CpanelResult.Data) != 1 ||
					created.CpanelResult.Data[0].LineKey != "created-line-key" {
					t.Fatalf("CreateCronJob() = %#v", created)
				}

				return nil
			},
		},
		{
			name:     "update",
			function: OperationEditLine,
			parameters: url.Values{
				"linekey": {"updated-line-key"},
				"command": {command},
				"minute":  {"11"},
				"hour":    {"4"},
				"day":     {"20"},
				"weekday": {"5"},
				"month":   {"9"},
			},
			call: func(client *Client) error {
				updated, err := client.UpdateCronJob(
					context.Background(),
					CronJobUpdateModel{
						LineKey: "updated-line-key",
						CronJobDetailsModel: CronJobDetailsModel{
							Command: command,
							Minute:  "11",
							Hour:    "4",
							Day:     "20",
							Weekday: "5",
							Month:   "9",
						},
					},
				)
				if err != nil {
					return err
				}
				if len(updated.CpanelResult.Data) != 1 ||
					updated.CpanelResult.Data[0].LineKey != "created-line-key" {
					t.Fatalf("UpdateCronJob() = %#v", updated)
				}

				return nil
			},
		},
		{
			name:       "delete",
			function:   OperationRemoveLine,
			parameters: url.Values{"line": {"3"}},
			call: func(client *Client) error {
				deleted, err := client.DeleteCronJob(
					context.Background(),
					CronJobDeleteModel{LineKey: "deleted-line-key"},
				)
				if err != nil {
					return err
				}
				if len(deleted.CpanelResult.Data) != 1 ||
					deleted.CpanelResult.Data[0].Status != 1 {
					t.Fatalf("DeleteCronJob() = %#v", deleted)
				}

				return nil
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if testCase.name == "delete" &&
					request.Method == http.MethodGet {
					assertCronAPI2Request(
						t,
						request,
						http.MethodGet,
						OperationFetchCron,
						url.Values{},
					)
					writeCronJSON(
						t,
						response,
						`{"cpanelresult":{"apiversion":2,"func":"fetchcron","module":"Cron","event":{"result":1},"data":[{"linekey":"mail-line-key","line":1,"value":"user@example.com","type":"variable","key":"MAILTO"},{"linekey":"shell-line-key","line":2,"value":"/bin/bash","type":"variable","key":"SHELL"},{"linekey":"deleted-line-key","line":7,"commandnumber":3,"command":"/usr/bin/true","type":"command"}]}}`,
					)
					return
				}
				if testCase.name == "create" &&
					request.Method == http.MethodGet {
					assertCronAPI2Request(
						t,
						request,
						http.MethodGet,
						OperationFetchCron,
						url.Values{},
					)
					writeCronJSON(
						t,
						response,
						`{"cpanelresult":{"event":{"result":1},"data":[]}}`,
					)
					return
				}
				assertCronAPI2Request(
					t,
					request,
					http.MethodPost,
					testCase.function,
					testCase.parameters,
				)
				if strings.Contains(request.URL.String(), command) {
					t.Errorf(
						"request URL contains command: %s",
						request.URL.Redacted(),
					)
				}

				writeCronJSON(
					t,
					response,
					`{"cpanelresult":{"apiversion":2,"func":"mutation","module":"Cron","event":{"result":1},"data":[{"linekey":"created-line-key","statusmsg":"success","status":1,"reason":"","result":1}]}}`,
				)
			}))
			defer server.Close()

			if err := testCase.call(newCronTestClient(t, server.URL)); err != nil {
				t.Fatalf("%s error: %v", testCase.name, err)
			}
		})
	}
}

func TestClientReconcilesAmbiguousCronCreation(t *testing.T) {
	t.Parallel()

	details := CronJobDetailsModel{
		Command: "/usr/bin/true",
		Minute:  "7",
		Hour:    "3",
		Day:     "15",
		Weekday: "2",
		Month:   "6",
	}
	fetchCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch request.Form.Get("cpanel_jsonapi_func") {
		case OperationFetchCron:
			fetchCalls++
			data := "[]"
			if fetchCalls == 2 {
				data = `[{"linekey":"created-line-key","line":7,"command":"/usr/bin/true","minute":"7","hour":"3","day":"15","weekday":"2","month":"6","type":"command"}]`
			}
			writeCronJSON(
				t,
				response,
				`{"cpanelresult":{"event":{"result":1},"data":`+data+`}}`,
			)
		case OperationAddLine:
			_, _ = response.Write([]byte("{"))
		default:
			t.Fatalf(
				"unexpected cron operation %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
	defer server.Close()

	created, err := newCronTestClient(t, server.URL).CreateCronJob(
		t.Context(),
		CronJobCreateModel{CronJobDetailsModel: details},
	)
	if err != nil {
		t.Fatalf("CreateCronJob() error: %v", err)
	}
	if len(created.CpanelResult.Data) != 1 ||
		created.CpanelResult.Data[0].LineKey != "created-line-key" ||
		created.CpanelResult.Data[0].Status != 1 {
		t.Fatalf("CreateCronJob() = %#v", created)
	}
}

func TestClientReconcilesAmbiguousCronUpdate(t *testing.T) {
	t.Parallel()

	previous := CronJobDetailsModel{
		Command: "/usr/bin/true",
		Minute:  "7",
		Hour:    "3",
		Day:     "15",
		Weekday: "2",
		Month:   "6",
	}
	desired := CronJobDetailsModel{
		Command: "/usr/bin/false",
		Minute:  "11",
		Hour:    "4",
		Day:     "20",
		Weekday: "5",
		Month:   "9",
	}
	testCases := map[string]struct {
		after     string
		wantKey   CronLineKey
		wantError error
	}{
		"line key is preserved": {
			after:   cronJobJSON("managed-line-key", desired),
			wantKey: "managed-line-key",
		},
		"new line key is attributed": {
			after: cronJobJSON("existing-match", desired) + "," +
				cronJobJSON("updated-line-key", desired),
			wantKey: "updated-line-key",
		},
		"multiple new line keys are rejected": {
			after: cronJobJSON("existing-match", desired) + "," +
				cronJobJSON("updated-line-key", desired) + "," +
				cronJobJSON("second-updated-line-key", desired),
			wantError: ErrCronJobUpdateAmbiguous,
		},
		"missing requested state is rejected": {},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fetchCalls := 0
			editCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				if err := request.ParseForm(); err != nil {
					t.Fatalf("ParseForm() error: %v", err)
				}
				switch request.Form.Get("cpanel_jsonapi_func") {
				case OperationFetchCron:
					fetchCalls++
					data := cronJobJSON("managed-line-key", previous) + "," +
						cronJobJSON("existing-match", desired)
					if fetchCalls > 1 {
						data = testCase.after
					}
					writeCronJSON(
						t,
						response,
						`{"cpanelresult":{"event":{"result":1},"data":[`+data+`]}}`,
					)
				case OperationEditLine:
					editCalls++
					_, _ = response.Write([]byte("{"))
				default:
					t.Fatalf(
						"unexpected cron operation %q",
						request.Form.Get("cpanel_jsonapi_func"),
					)
				}
			}))
			defer server.Close()

			updated, err := newCronTestClient(t, server.URL).UpdateCronJob(
				t.Context(),
				CronJobUpdateModel{
					LineKey:             "managed-line-key",
					CronJobDetailsModel: desired,
					Expected:            &previous,
				},
			)
			if testCase.wantError != nil {
				if !errors.Is(err, testCase.wantError) {
					t.Fatalf("UpdateCronJob() error = %v, want %v", err, testCase.wantError)
				}
			} else if testCase.wantKey == "" {
				if err == nil {
					t.Fatal("UpdateCronJob() error = nil, want ambiguous failure")
				}
			} else {
				if err != nil {
					t.Fatalf("UpdateCronJob() error: %v", err)
				}
				if len(updated.CpanelResult.Data) != 1 ||
					updated.CpanelResult.Data[0].LineKey != testCase.wantKey {
					t.Fatalf("UpdateCronJob() = %#v, want line key %q", updated, testCase.wantKey)
				}
			}
			if fetchCalls != 2 {
				t.Fatalf("fetch calls = %d, want 2", fetchCalls)
			}
			if editCalls != 1 {
				t.Fatalf("edit calls = %d, want 1", editCalls)
			}
		})
	}
}

func cronJobJSON(lineKey string, details CronJobDetailsModel) string {
	body, err := json.Marshal(map[string]any{
		"linekey": lineKey,
		"line":    7,
		"command": details.Command,
		"minute":  details.Minute,
		"hour":    details.Hour,
		"day":     details.Day,
		"weekday": details.Weekday,
		"month":   details.Month,
		"type":    "command",
	})
	if err != nil {
		panic(err)
	}

	return string(body)
}

func TestClientRefusesChangedCronDeletion(t *testing.T) {
	t.Parallel()

	expected := CronJobDetailsModel{
		Command: "/usr/bin/true",
		Minute:  "7",
		Hour:    "3",
		Day:     "15",
		Weekday: "2",
		Month:   "6",
	}
	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch request.Form.Get("cpanel_jsonapi_func") {
		case OperationFetchCron:
			writeCronJSON(
				t,
				response,
				`{"cpanelresult":{"event":{"result":1},"data":[{"linekey":"managed-line-key","line":7,"command":"/usr/bin/false","minute":"7","hour":"3","day":"15","weekday":"2","month":"6","type":"command"}]}}`,
			)
		case OperationRemoveLine:
			deleteCalls++
		default:
			t.Fatalf(
				"unexpected cron operation %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
	defer server.Close()

	_, err := newCronTestClient(t, server.URL).DeleteCronJob(
		t.Context(),
		CronJobDeleteModel{
			LineKey:  "managed-line-key",
			Expected: &expected,
		},
	)
	if !errors.Is(err, ErrCronJobChanged) {
		t.Fatalf("DeleteCronJob() error = %v, want ErrCronJobChanged", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", deleteCalls)
	}
}

func TestClientRefusesCronDeletionWithoutCommandNumber(t *testing.T) {
	t.Parallel()

	deleteCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		switch request.Form.Get("cpanel_jsonapi_func") {
		case OperationFetchCron:
			writeCronJSON(
				t,
				response,
				`{"cpanelresult":{"event":{"result":1},"data":[{"linekey":"managed-line-key","line":3,"command":"/usr/bin/true","commandnumber":0,"type":"command"}]}}`,
			)
		case OperationRemoveLine:
			deleteCalls++
		default:
			t.Fatalf(
				"unexpected cron operation %q",
				request.Form.Get("cpanel_jsonapi_func"),
			)
		}
	}))
	defer server.Close()

	_, err := newCronTestClient(t, server.URL).DeleteCronJob(
		t.Context(),
		CronJobDeleteModel{LineKey: "managed-line-key"},
	)
	if err == nil ||
		!strings.Contains(err.Error(), "invalid command number 0") {
		t.Fatalf("DeleteCronJob() error = %v", err)
	}
	if deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", deleteCalls)
	}
}

func TestClientPropagatesCronAPIErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeCronJSON(
			t,
			response,
			`{"cpanelresult":{"event":{"result":0},"data":[{"reason":"cron entry not found","statusmsg":"failed"}]}}`,
		)
	}))
	defer server.Close()

	_, err := newCronTestClient(t, server.URL).DeleteCronJob(
		t.Context(),
		CronJobDeleteModel{LineKey: "missing-line-key"},
	)
	var apiError *cpanel.APIError
	if !errors.As(err, &apiError) {
		t.Fatalf("DeleteCronJob() error = %T %v, want *cpanel.APIError", err, err)
	}
	if !strings.Contains(err.Error(), "cron entry not found; failed") {
		t.Fatalf("DeleteCronJob() error = %v", err)
	}
}

func TestClientTreatsMissingCronJobAsAlreadyDeleted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		assertCronAPI2Request(
			t,
			request,
			http.MethodGet,
			OperationFetchCron,
			url.Values{},
		)
		writeCronJSON(
			t,
			response,
			`{"cpanelresult":{"event":{"result":1},"data":[]}}`,
		)
	}))
	defer server.Close()

	deleted, err := newCronTestClient(t, server.URL).DeleteCronJob(
		t.Context(),
		CronJobDeleteModel{LineKey: "missing-line-key"},
	)
	if err != nil {
		t.Fatalf("DeleteCronJob() error: %v", err)
	}
	if deleted != nil {
		t.Fatalf("DeleteCronJob() = %#v, want nil", deleted)
	}
}

func TestClientRejectsMalformedCronResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		_ *http.Request,
	) {
		writeCronJSON(t, response, `{"cpanelresult":`)
	}))
	defer server.Close()

	_, err := newCronTestClient(t, server.URL).GetCronJobs(t.Context())
	if err == nil || !strings.Contains(err.Error(), "decode API 2 response envelope") {
		t.Fatalf("GetCronJobs() error = %v", err)
	}
}

func assertCronAPI2Request(
	t *testing.T,
	request *http.Request,
	method string,
	function string,
	expected url.Values,
) {
	t.Helper()

	if request.Method != method {
		t.Errorf("method = %s, want %s", request.Method, method)
	}
	if request.URL.Path != "/json-api/cpanel" {
		t.Errorf("path = %s, want /json-api/cpanel", request.URL.Path)
	}
	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	if got := request.Form.Get("cpanel_jsonapi_apiversion"); got != "2" {
		t.Errorf("cpanel_jsonapi_apiversion = %q, want 2", got)
	}
	if got := request.Form.Get("cpanel_jsonapi_user"); got != "username" {
		t.Errorf("cpanel_jsonapi_user = %q, want username", got)
	}
	if got := request.Form.Get("cpanel_jsonapi_module"); got != "Cron" {
		t.Errorf("cpanel_jsonapi_module = %q, want Cron", got)
	}
	if got := request.Form.Get("cpanel_jsonapi_func"); got != function {
		t.Errorf("cpanel_jsonapi_func = %q, want %q", got, function)
	}

	actual := url.Values{}
	for key, values := range request.Form {
		actual[key] = append([]string(nil), values...)
	}
	for _, key := range []string{
		"cpanel_jsonapi_apiversion",
		"cpanel_jsonapi_user",
		"cpanel_jsonapi_module",
		"cpanel_jsonapi_func",
	} {
		actual.Del(key)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("parameters = %#v, want %#v", actual, expected)
	}
}

func writeCronJSON(
	t *testing.T,
	response http.ResponseWriter,
	body string,
) {
	t.Helper()

	response.Header().Set("Content-Type", "application/json")
	if _, err := response.Write([]byte(body)); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func newCronTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
