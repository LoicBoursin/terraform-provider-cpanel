package dns

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientParsesZoneRecords(t *testing.T) {
	t.Parallel()

	encode := base64.StdEncoding.EncodeToString
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path != "/execute/DNS/parse_zone" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if got := request.URL.Query().Get("zone"); got != "example.test" {
			t.Errorf("zone = %q, want example.test", got)
		}

		_, _ = fmt.Fprintf(
			response,
			`{"status":1,"data":[`+
				`{"line_index":0,"type":"$TTL","text_b64":"%s"},`+
				`{"line_index":3,"record_type":"SOA","ttl":86400,"dname_b64":"%s","data_b64":["%s","%s","%s","%s","%s","%s","%s"]},`+
				`{"line_index":11,"record_type":"A","ttl":300,"dname_b64":"%s","data_b64":["%s"]}`+
				`]}`,
			encode([]byte("$TTL 14400")),
			encode([]byte("example.test.")),
			encode([]byte("ns1.example.test.")),
			encode([]byte("hostmaster.example.test.")),
			encode([]byte("2026071401")),
			encode([]byte("3600")),
			encode([]byte("1800")),
			encode([]byte("1209600")),
			encode([]byte("86400")),
			encode([]byte("www.example.test.")),
			encode([]byte("192.0.2.10")),
		)
	}))
	defer server.Close()

	client := newDNSTestClient(t, server.URL)
	zone, err := client.ParseZone(context.Background(), "example.test")
	if err != nil {
		t.Fatalf("ParseZone() error: %v", err)
	}
	if zone.Serial != 2026071401 {
		t.Fatalf("zone serial = %d, want 2026071401", zone.Serial)
	}
	record := zone.RecordAt(11)
	if record == nil {
		t.Fatal("record at line 11 was not found")
	}
	if record.Name != "www" || record.Type != "A" ||
		record.TTL != 300 || len(record.Data) != 1 ||
		record.Data[0] != "192.0.2.10" {
		t.Fatalf("record = %#v", record)
	}
}

func TestClientAddsDNSRecordWithPOST(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		if request.URL.Path == "/execute/DNS/parse_zone" {
			encode := base64.StdEncoding.EncodeToString
			_, _ = fmt.Fprintf(
				response,
				`{"status":1,"data":[{"line_index":3,"record_type":"SOA","ttl":86400,"dname_b64":"%s","data_b64":["%s","%s","%s","%s","%s","%s","%s"]}]}`,
				encode([]byte("example.test.")),
				encode([]byte("ns1.example.test.")),
				encode([]byte("hostmaster.example.test.")),
				encode([]byte("2026071401")),
				encode([]byte("3600")),
				encode([]byte("1800")),
				encode([]byte("1209600")),
				encode([]byte("86400")),
			)
			return
		}

		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", request.Method)
		}
		if request.URL.Path != "/execute/DNS/mass_edit_zone" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.URL.RawQuery != "" {
			t.Errorf("request URL query = %q, want empty", request.URL.RawQuery)
		}
		if err := request.ParseForm(); err != nil {
			t.Fatalf("ParseForm() error: %v", err)
		}
		if got := request.Form.Get("zone"); got != "example.test" {
			t.Errorf("zone = %q, want example.test", got)
		}
		if got := request.Form.Get("serial"); got != "2026071401" {
			t.Errorf("serial = %q, want 2026071401", got)
		}

		var mutation recordMutation
		if err := json.Unmarshal([]byte(request.Form.Get("add")), &mutation); err != nil {
			t.Fatalf("decode add mutation: %v", err)
		}
		if mutation.Name != "example.test." ||
			mutation.RecordType != "TXT" ||
			mutation.TTL != 300 ||
			len(mutation.Data) != 1 ||
			mutation.Data[0] != "terraform-provider-cpanel" {
			t.Fatalf("mutation = %#v", mutation)
		}

		_, _ = response.Write([]byte(
			`{"status":1,"data":{"new_serial":"2026071402"}}`,
		))
	}))
	defer server.Close()

	client := newDNSTestClient(t, server.URL)
	if err := client.AddRecord(
		context.Background(),
		"example.test",
		Record{
			Name: "@",
			Type: "TXT",
			TTL:  300,
			Data: []string{"terraform-provider-cpanel"},
		},
	); err != nil {
		t.Fatalf("AddRecord() error: %v", err)
	}
}

func TestClientReconcilesAmbiguousDNSRecordAddition(t *testing.T) {
	t.Parallel()

	record := Record{
		Name: "@",
		Type: "TXT",
		TTL:  300,
		Data: []string{"terraform-provider-cpanel"},
	}
	testCases := map[string]struct {
		recordsAfterMutation int
		wantError            error
	}{
		"exact record appears once": {
			recordsAfterMutation: 1,
		},
		"record remains absent": {},
		"duplicate exact records": {
			recordsAfterMutation: 2,
			wantError:            ErrRecordAmbiguous,
		},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parseCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/DNS/parse_zone":
					parseCalls++
					writeDNSZoneResponse(
						t,
						response,
						2026071400+int64(parseCalls),
						record,
						map[bool]int{
							true:  test.recordsAfterMutation,
							false: 0,
						}[parseCalls > 1],
					)
				case "/execute/DNS/mass_edit_zone":
					response.WriteHeader(http.StatusGatewayTimeout)
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			err := newDNSTestClient(t, server.URL).AddRecord(
				t.Context(),
				"example.test",
				record,
			)
			if test.wantError != nil {
				if !errors.Is(err, test.wantError) {
					t.Fatalf("AddRecord() error = %v, want %v", err, test.wantError)
				}
			} else if test.recordsAfterMutation == 0 {
				if err == nil {
					t.Fatal("AddRecord() error = nil, want ambiguous failure")
				}
			} else if err != nil {
				t.Fatalf("AddRecord() error = %v", err)
			}
			if parseCalls != 2 {
				t.Fatalf("parse zone calls = %d, want 2", parseCalls)
			}
		})
	}
}

func TestClientDoesNotReconcileDeterministicDNSRecordAdditionError(
	t *testing.T,
) {
	t.Parallel()

	parseCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(
		response http.ResponseWriter,
		request *http.Request,
	) {
		switch request.URL.Path {
		case "/execute/DNS/parse_zone":
			parseCalls++
			writeDNSZoneResponse(
				t,
				response,
				2026071401,
				Record{},
				0,
			)
		case "/execute/DNS/mass_edit_zone":
			response.WriteHeader(http.StatusBadRequest)
		default:
			t.Fatalf(
				"unexpected request: %s %s",
				request.Method,
				request.URL.Path,
			)
		}
	}))
	defer server.Close()

	err := newDNSTestClient(t, server.URL).AddRecord(
		t.Context(),
		"example.test",
		Record{
			Name: "@",
			Type: "TXT",
			TTL:  300,
			Data: []string{"terraform-provider-cpanel"},
		},
	)
	if err == nil {
		t.Fatal("AddRecord() error = nil")
	}
	if parseCalls != 1 {
		t.Fatalf("parse zone calls = %d, want 1", parseCalls)
	}
}

func TestClientReconcilesAmbiguousDNSRecordUpdate(t *testing.T) {
	t.Parallel()

	current := Record{
		LineIndex: 11,
		Name:      "www",
		Type:      "A",
		TTL:       300,
		Data:      []string{"192.0.2.10"},
	}
	desired := Record{
		Name: "www",
		Type: "A",
		TTL:  600,
		Data: []string{"192.0.2.20"},
	}
	testCases := map[string]struct {
		records   []Record
		wantLine  int64
		wantError error
	}{
		"requested record appears once": {
			records: []Record{{
				LineIndex: 14,
				Name:      desired.Name,
				Type:      desired.Type,
				TTL:       desired.TTL,
				Data:      desired.Data,
			}},
			wantLine: 14,
		},
		"previous record remains": {
			records: []Record{current},
		},
		"previous and requested records coexist": {
			records: []Record{
				current,
				{
					LineIndex: 14,
					Name:      desired.Name,
					Type:      desired.Type,
					TTL:       desired.TTL,
					Data:      desired.Data,
				},
			},
		},
		"requested record is duplicated": {
			records: []Record{
				{
					LineIndex: 14,
					Name:      desired.Name,
					Type:      desired.Type,
					TTL:       desired.TTL,
					Data:      desired.Data,
				},
				{
					LineIndex: 15,
					Name:      desired.Name,
					Type:      desired.Type,
					TTL:       desired.TTL,
					Data:      desired.Data,
				},
			},
			wantError: ErrRecordAmbiguous,
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			parseCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(
				response http.ResponseWriter,
				request *http.Request,
			) {
				switch request.URL.Path {
				case "/execute/DNS/parse_zone":
					parseCalls++
					records := []Record{current}
					if parseCalls > 1 {
						records = testCase.records
					}
					writeDNSZoneRecordsResponse(
						t,
						response,
						2026071400+int64(parseCalls),
						records,
					)
				case "/execute/DNS/mass_edit_zone":
					response.WriteHeader(http.StatusGatewayTimeout)
				default:
					t.Fatalf(
						"unexpected request: %s %s",
						request.Method,
						request.URL.Path,
					)
				}
			}))
			defer server.Close()

			lineIndex, err := newDNSTestClient(t, server.URL).UpdateRecord(
				t.Context(),
				"example.test",
				current,
				desired,
			)
			if testCase.wantError != nil {
				if !errors.Is(err, testCase.wantError) {
					t.Fatalf("UpdateRecord() error = %v, want %v", err, testCase.wantError)
				}
			} else if testCase.wantLine == 0 {
				if err == nil {
					t.Fatal("UpdateRecord() error = nil, want ambiguous failure")
				}
			} else {
				if err != nil {
					t.Fatalf("UpdateRecord() error: %v", err)
				}
				if lineIndex != testCase.wantLine {
					t.Fatalf("UpdateRecord() line index = %d, want %d", lineIndex, testCase.wantLine)
				}
			}
			if parseCalls != 2 {
				t.Fatalf("parse zone calls = %d, want 2", parseCalls)
			}
		})
	}
}

func writeDNSZoneResponse(
	t *testing.T,
	response http.ResponseWriter,
	serial int64,
	record Record,
	recordCount int,
) {
	t.Helper()

	records := make([]Record, 0, recordCount)
	for index := 0; index < recordCount; index++ {
		record.LineIndex = int64(11 + index)
		records = append(records, record)
	}
	writeDNSZoneRecordsResponse(t, response, serial, records)
}

func writeDNSZoneRecordsResponse(
	t *testing.T,
	response http.ResponseWriter,
	serial int64,
	records []Record,
) {
	t.Helper()

	encode := base64.StdEncoding.EncodeToString
	entries := []map[string]any{{
		"line_index":  3,
		"record_type": "SOA",
		"ttl":         86400,
		"dname_b64":   encode([]byte("example.test.")),
		"data_b64": []string{
			encode([]byte("ns1.example.test.")),
			encode([]byte("hostmaster.example.test.")),
			encode([]byte(fmt.Sprintf("%d", serial))),
			encode([]byte("3600")),
			encode([]byte("1800")),
			encode([]byte("1209600")),
			encode([]byte("86400")),
		},
	}}
	for _, record := range records {
		name := record.Name
		if name == "@" {
			name = "example.test."
		}
		data := make([]string, 0, len(record.Data))
		for _, value := range record.Data {
			data = append(data, encode([]byte(value)))
		}
		entries = append(entries, map[string]any{
			"line_index":  record.LineIndex,
			"record_type": record.Type,
			"ttl":         record.TTL,
			"dname_b64":   encode([]byte(name)),
			"data_b64":    data,
		})
	}

	if err := json.NewEncoder(response).Encode(map[string]any{
		"status": 1,
		"data":   entries,
	}); err != nil {
		t.Fatalf("encode DNS zone response: %v", err)
	}
}

func TestZoneLocateRecordRequiresExactMatch(t *testing.T) {
	t.Parallel()

	expected := Record{
		LineIndex: 11,
		Name:      "www",
		Type:      "A",
		TTL:       300,
		Data:      []string{"192.0.2.10"},
	}

	tests := map[string]struct {
		records   []Record
		wantLine  int64
		wantError error
	}{
		"exact line": {
			records:  []Record{expected},
			wantLine: 11,
		},
		"moved exact record": {
			records: []Record{
				{
					LineIndex: 11,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.20"},
				},
				{
					LineIndex: 14,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.10"},
				},
			},
			wantLine: 14,
		},
		"same identity with different value": {
			records: []Record{{
				LineIndex: 11,
				Name:      "www",
				Type:      "A",
				TTL:       300,
				Data:      []string{"192.0.2.20"},
			}},
			wantError: ErrRecordNotFound,
		},
		"duplicate exact records": {
			records: []Record{
				expected,
				{
					LineIndex: 14,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.10"},
				},
			},
			wantLine: 11,
		},
		"duplicate exact records after line reuse": {
			records: []Record{
				{
					LineIndex: 11,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.20"},
				},
				{
					LineIndex: 14,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.10"},
				},
				{
					LineIndex: 15,
					Name:      "www",
					Type:      "A",
					TTL:       300,
					Data:      []string{"192.0.2.10"},
				},
			},
			wantError: ErrRecordAmbiguous,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			record, err := (&Zone{Records: test.records}).LocateRecord(expected)
			if !errors.Is(err, test.wantError) {
				t.Fatalf("LocateRecord() error = %v, want %v", err, test.wantError)
			}
			if test.wantError != nil {
				return
			}
			if record == nil || record.LineIndex != test.wantLine {
				t.Fatalf("LocateRecord() = %#v, want line %d", record, test.wantLine)
			}
		})
	}
}

func TestZoneLocateManagedRecordAllowsObservableDrift(t *testing.T) {
	t.Parallel()

	expected := Record{
		LineIndex: 11,
		Name:      "www",
		Type:      "TXT",
		TTL:       300,
		Data:      []string{"managed"},
	}
	drifted := expected
	drifted.TTL = 600
	drifted.Data = []string{"external"}

	tests := map[string]struct {
		records   []Record
		wantLine  int64
		wantError error
	}{
		"drift at saved line": {
			records:  []Record{drifted},
			wantLine: 11,
		},
		"moved exact record": {
			records: []Record{
				{
					LineIndex: 11,
					Name:      "other",
					Type:      "TXT",
					TTL:       300,
					Data:      []string{"other"},
				},
				{
					LineIndex: 14,
					Name:      expected.Name,
					Type:      expected.Type,
					TTL:       expected.TTL,
					Data:      expected.Data,
				},
			},
			wantLine: 14,
		},
		"single moved drifted record": {
			records: []Record{{
				LineIndex: 14,
				Name:      drifted.Name,
				Type:      drifted.Type,
				TTL:       drifted.TTL,
				Data:      drifted.Data,
			}},
			wantLine: 14,
		},
		"ambiguous moved identity": {
			records: []Record{
				{
					LineIndex: 14,
					Name:      drifted.Name,
					Type:      drifted.Type,
					TTL:       drifted.TTL,
					Data:      drifted.Data,
				},
				{
					LineIndex: 15,
					Name:      drifted.Name,
					Type:      drifted.Type,
					TTL:       900,
					Data:      []string{"another"},
				},
			},
			wantError: ErrRecordAmbiguous,
		},
		"missing identity": {
			wantError: ErrRecordNotFound,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			record, err := (&Zone{Records: test.records}).LocateManagedRecord(
				expected,
			)
			if !errors.Is(err, test.wantError) {
				t.Fatalf(
					"LocateManagedRecord() error = %v, want %v",
					err,
					test.wantError,
				)
			}
			if test.wantError != nil {
				return
			}
			if record == nil || record.LineIndex != test.wantLine {
				t.Fatalf(
					"LocateManagedRecord() = %#v, want line %d",
					record,
					test.wantLine,
				)
			}
		})
	}
}

func newDNSTestClient(t *testing.T, host string) *Client {
	t.Helper()

	baseClient, err := cpanel.NewClient(host, "username", "api-token")
	if err != nil {
		t.Fatalf("cpanel.NewClient() error: %v", err)
	}

	return NewClient(baseClient)
}
