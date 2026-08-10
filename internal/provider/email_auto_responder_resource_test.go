package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"terraform-provider-cpanel/internal/cpanel"
	cpanelmail "terraform-provider-cpanel/internal/cpanel/email"
)

func TestEmailAutoResponderRestoreRequiresAttemptedState(t *testing.T) {
	t.Parallel()

	original := emailAutoResponderTestDefinition("Original")
	attempted := emailAutoResponderTestDefinition("Attempted")
	concurrent := emailAutoResponderTestDefinition("Concurrent")

	testCases := []struct {
		name         string
		initial      *cpanelmail.AutoResponder
		wantError    bool
		wantCurrent  *cpanelmail.AutoResponder
		wantSetCalls int
	}{
		{
			name:         "attempted state is restored",
			initial:      &attempted,
			wantCurrent:  &original,
			wantSetCalls: 1,
		},
		{
			name:        "original already restored is accepted",
			initial:     &original,
			wantCurrent: &original,
		},
		{
			name:        "concurrent state is preserved",
			initial:     &concurrent,
			wantError:   true,
			wantCurrent: &concurrent,
		},
		{
			name:      "absent attempted state is not recreated over",
			wantError: true,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := testCase.initial
			setCalls := 0
			server := newEmailAutoResponderResourceTestServer(
				t,
				&current,
				func(response http.ResponseWriter, request *http.Request) {
					setCalls++
					updated := autoResponderFromTestRequest(t, request)
					current = &updated
					writeEmailAutoResponderTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous update"},
						"data":   nil,
					})
				},
				nil,
			)
			defer server.Close()

			resource := emailAutoResponderTestResource(t, server.URL)
			err := resource.restoreAutoResponder(
				t.Context(),
				"away",
				"example.test",
				attempted,
				original,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"restoreAutoResponder() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if !autoResponderPointersEqual(current, testCase.wantCurrent) {
				t.Fatalf(
					"current = %#v, want %#v",
					current,
					testCase.wantCurrent,
				)
			}
			if setCalls != testCase.wantSetCalls {
				t.Fatalf(
					"setCalls = %d, want %d",
					setCalls,
					testCase.wantSetCalls,
				)
			}
		})
	}
}

func TestEmailAutoResponderRollbackCreatedPreservesConcurrentState(
	t *testing.T,
) {
	t.Parallel()

	attempted := emailAutoResponderTestDefinition("Attempted")
	concurrent := emailAutoResponderTestDefinition("Concurrent")

	testCases := []struct {
		name            string
		initial         *cpanelmail.AutoResponder
		wantError       bool
		wantCurrent     *cpanelmail.AutoResponder
		wantDeleteCalls int
	}{
		{
			name:            "attempted creation is removed",
			initial:         &attempted,
			wantDeleteCalls: 1,
		},
		{
			name:        "concurrent state is preserved",
			initial:     &concurrent,
			wantError:   true,
			wantCurrent: &concurrent,
		},
		{
			name: "already absent is accepted",
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			current := testCase.initial
			deleteCalls := 0
			server := newEmailAutoResponderResourceTestServer(
				t,
				&current,
				nil,
				func(response http.ResponseWriter, _ *http.Request) {
					deleteCalls++
					current = nil
					writeEmailAutoResponderTestJSON(t, response, map[string]any{
						"status": 0,
						"errors": []string{"ambiguous delete"},
						"data":   nil,
					})
				},
			)
			defer server.Close()

			resource := emailAutoResponderTestResource(t, server.URL)
			err := resource.rollbackCreatedAutoResponder(
				t.Context(),
				"example.test",
				attempted,
			)
			if (err != nil) != testCase.wantError {
				t.Fatalf(
					"rollbackCreatedAutoResponder() error = %v, wantError %t",
					err,
					testCase.wantError,
				)
			}
			if !autoResponderPointersEqual(current, testCase.wantCurrent) {
				t.Fatalf(
					"current = %#v, want %#v",
					current,
					testCase.wantCurrent,
				)
			}
			if deleteCalls != testCase.wantDeleteCalls {
				t.Fatalf(
					"deleteCalls = %d, want %d",
					deleteCalls,
					testCase.wantDeleteCalls,
				)
			}
		})
	}
}

func TestAccEmailAutoResponderResource(t *testing.T) {
	const resourceName = "cpanel_email_auto_responder.test"

	address := testAccEmailAutoResponderAddress(t, "resource")
	first := cpanelmail.AutoResponder{
		Email:    address,
		From:     "Terraform Provider",
		Subject:  "Automatic reply",
		Body:     "This message was sent automatically.",
		Charset:  "UTF-8",
		Interval: 8,
		IsHTML:   0,
	}
	second := cpanelmail.AutoResponder{
		Email:    address,
		From:     "Terraform Provider Updated",
		Subject:  "Updated automatic reply",
		Body:     "<p>This message was updated.</p>",
		Charset:  "UTF-8",
		Interval: 4,
		IsHTML:   1,
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: testAccCheckEmailAutoRespondersDestroyed(
			address,
		),
		Steps: []resource.TestStep{
			{
				Config: testAccEmailAutoResponderResourceConfig(first),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "email", address),
					resource.TestCheckResourceAttr(
						resourceName,
						"subject",
						first.Subject,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"interval_hours",
						"8",
					),
					resource.TestCheckResourceAttr(resourceName, "is_html", "false"),
					testAccCheckEmailAutoResponderExists(first),
				),
			},
			{
				ResourceName:                         resourceName,
				ImportStateId:                        address,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "email",
			},
			{
				Config: testAccEmailAutoResponderResourceConfig(second),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(
						resourceName,
						"subject",
						second.Subject,
					),
					resource.TestCheckResourceAttr(
						resourceName,
						"interval_hours",
						"4",
					),
					resource.TestCheckResourceAttr(resourceName, "is_html", "true"),
					testAccCheckEmailAutoResponderExists(second),
				),
			},
			{
				PreConfig: func() {
					testAccDeleteEmailAutoResponder(t, address)
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config: testAccEmailAutoResponderResourceConfig(second),
				Check:  testAccCheckEmailAutoResponderExists(second),
			},
		},
	})
}

func emailAutoResponderTestDefinition(label string) cpanelmail.AutoResponder {
	start := int64(1_800_000_000)
	stop := int64(1_800_086_400)

	return cpanelmail.AutoResponder{
		Email:    "away@example.test",
		From:     label,
		Subject:  label + " subject",
		Body:     label + " body",
		Charset:  "UTF-8",
		Interval: 8,
		IsHTML:   0,
		Start:    &start,
		Stop:     &stop,
	}
}

func emailAutoResponderTestResource(
	t *testing.T,
	host string,
) emailAutoResponderResource {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return emailAutoResponderResource{
		client: cpanelmail.NewClient(baseClient),
	}
}

func newEmailAutoResponderResourceTestServer(
	t *testing.T,
	current **cpanelmail.AutoResponder,
	setHandler func(http.ResponseWriter, *http.Request),
	deleteHandler func(http.ResponseWriter, *http.Request),
) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/Email/list_auto_responders":
			data := []map[string]string{}
			if *current != nil {
				data = append(data, map[string]string{
					"email":   (*current).Email,
					"subject": (*current).Subject,
				})
			}
			writeEmailAutoResponderTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   data,
			})
		case "/execute/Email/get_auto_responder":
			if *current == nil {
				t.Fatal("requested details for absent autoresponder")
			}
			writeEmailAutoResponderTestJSON(t, response, map[string]any{
				"status": 1,
				"data":   *current,
			})
		case "/execute/Email/add_auto_responder":
			if setHandler == nil {
				t.Fatal("unexpected autoresponder update")
			}
			setHandler(response, request)
		case "/execute/Email/delete_auto_responder":
			if deleteHandler == nil {
				t.Fatal("unexpected autoresponder deletion")
			}
			deleteHandler(response, request)
		default:
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))
}

func autoResponderFromTestRequest(
	t *testing.T,
	request *http.Request,
) cpanelmail.AutoResponder {
	t.Helper()

	if err := request.ParseForm(); err != nil {
		t.Fatalf("ParseForm() error: %v", err)
	}
	start, err := strconv.ParseInt(request.Form.Get("start"), 10, 64)
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	stop, err := strconv.ParseInt(request.Form.Get("stop"), 10, 64)
	if err != nil {
		t.Fatalf("parse stop: %v", err)
	}
	interval, err := strconv.ParseInt(request.Form.Get("interval"), 10, 64)
	if err != nil {
		t.Fatalf("parse interval: %v", err)
	}
	isHTML, err := strconv.ParseInt(request.Form.Get("is_html"), 10, 64)
	if err != nil {
		t.Fatalf("parse is_html: %v", err)
	}

	return cpanelmail.AutoResponder{
		Email:    request.Form.Get("email") + "@" + request.Form.Get("domain"),
		From:     request.Form.Get("from"),
		Subject:  request.Form.Get("subject"),
		Body:     request.Form.Get("body"),
		Charset:  request.Form.Get("charset"),
		Interval: interval,
		IsHTML:   isHTML,
		Start:    &start,
		Stop:     &stop,
	}
}

func autoResponderPointersEqual(
	left *cpanelmail.AutoResponder,
	right *cpanelmail.AutoResponder,
) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}

	return autoRespondersEqual(*left, *right)
}

func writeEmailAutoResponderTestJSON(
	t *testing.T,
	response http.ResponseWriter,
	value any,
) {
	t.Helper()

	if err := json.NewEncoder(response).Encode(value); err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
}

func testAccEmailAutoResponderResourceConfig(
	autoResponder cpanelmail.AutoResponder,
) string {
	return providerConfig + fmt.Sprintf(`
resource "cpanel_email_auto_responder" "test" {
  email          = %q
  from           = %q
  subject        = %q
  body           = %q
  charset        = %q
  interval_hours = %d
  is_html        = %t
  start_unix     = %d
  stop_unix      = %d
}
`,
		autoResponder.Email,
		autoResponder.From,
		autoResponder.Subject,
		autoResponder.Body,
		autoResponder.Charset,
		autoResponder.Interval,
		autoResponder.IsHTML == 1,
		autoResponder.StartUnix(),
		autoResponder.StopUnix(),
	)
}
