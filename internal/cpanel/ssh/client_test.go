package ssh

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"terraform-provider-cpanel/internal/cpanel"
)

func TestClientReadsOnlyPublicSSHKeys(t *testing.T) {
	t.Parallel()

	publicKey := testPublicKey(t, "stored comment")
	state := &sshReadTestState{
		listData: json.RawMessage(
			`[{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub","auth":1,"authstatus":"authorized","authaction":"Deauthorize","ctime":1,"mtime":"2","haspub":1}]`,
		),
		fetched: map[string]string{"deploy": publicKey},
	}
	client := newSSHReadTestClient(t, state)

	inventory, err := client.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if !reflect.DeepEqual(inventory, []Metadata{{
		Name:       "deploy",
		Authorized: true,
		CreatedAt:  1,
		ModifiedAt: 2,
	}}) {
		t.Fatalf("List() = %#v", inventory)
	}

	key, err := client.Get(t.Context(), "deploy")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if key == nil ||
		key.Name != "deploy" ||
		!key.Authorized ||
		!strings.HasPrefix(key.FingerprintSHA256, "SHA256:") ||
		strings.Contains(key.PublicKey, "stored comment") {
		t.Fatalf("Get() = %#v", key)
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	if !reflect.DeepEqual(
		state.operations,
		[]sshReadOperation{
			{function: operationListKeys, method: http.MethodGet, public: "1"},
			{function: operationListKeys, method: http.MethodGet, public: "1"},
			{function: operationFetchKey, method: http.MethodGet, public: "1"},
		},
	) {
		t.Fatalf("operations = %#v", state.operations)
	}
}

func TestClientAcceptsEmptyPublicInventory(t *testing.T) {
	t.Parallel()

	state := &sshReadTestState{
		listData: json.RawMessage(`[]`),
	}
	inventory, err := newSSHReadTestClient(t, state).List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if inventory == nil || len(inventory) != 0 {
		t.Fatalf("List() = %#v; expected non-nil empty inventory", inventory)
	}
}

func TestClientAcceptsDocumentedAndObservedAuthorizationFormats(
	t *testing.T,
) {
	t.Parallel()

	tests := map[string]struct {
		fields     string
		authorized bool
	}{
		"observed authorized strings": {
			fields:     `"auth":1,"authstatus":"authorized","authaction":"Deauthorize"`,
			authorized: true,
		},
		"observed deauthorized strings": {
			fields:     `"auth":"0","authstatus":"deauthorized","authaction":"Authorize"`,
			authorized: false,
		},
		"documented booleans": {
			fields:     `"auth":true,"authstatus":true,"authaction":false`,
			authorized: true,
		},
		"documented numeric status": {
			fields:     `"auth":null,"authstatus":0,"authaction":0`,
			authorized: false,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := &sshReadTestState{
				listData: json.RawMessage(fmt.Sprintf(
					`[{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub",%s,"ctime":1,"mtime":2,"haspub":1,"future_field":"ignored"}]`,
					test.fields,
				)),
			}
			inventory, err := newSSHReadTestClient(t, state).
				List(t.Context())
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if len(inventory) != 1 ||
				inventory[0].Authorized != test.authorized {
				t.Fatalf("List() = %#v", inventory)
			}
		})
	}
}

func TestClientRejectsContradictoryAuthorizationFields(t *testing.T) {
	t.Parallel()

	state := &sshReadTestState{
		listData: json.RawMessage(
			`[{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub","auth":1,"authstatus":"deauthorized","ctime":1,"mtime":2}]`,
		),
	}
	_, err := newSSHReadTestClient(t, state).List(t.Context())
	if err == nil || !strings.Contains(err.Error(), "disagree") {
		t.Fatalf("List() error = %v", err)
	}
}

func TestClientRejectsMalformedPublicInventory(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing authorization": `{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub","ctime":1,"mtime":2}`,
		"private filename":      `{"name":"deploy","key":"deploy","file":"/home/test-user/.ssh/deploy","auth":0,"ctime":1,"mtime":2}`,
		"invalid timestamp":     `{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub","auth":0,"ctime":"yesterday","mtime":2}`,
	}
	for name, item := range tests {
		item := item
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			state := &sshReadTestState{
				listData: json.RawMessage("[" + item + "]"),
			}
			if _, err := newSSHReadTestClient(t, state).
				List(t.Context()); err == nil {
				t.Fatal("List() returned no error")
			}
		})
	}
}

func TestClientRejectsFetchedPrivateMaterial(t *testing.T) {
	t.Parallel()

	state := &sshReadTestState{
		listData: json.RawMessage(
			`[{"name":"deploy","key":"deploy.pub","file":"/home/test-user/.ssh/deploy.pub","auth":0,"ctime":1,"mtime":2}]`,
		),
		fetched: map[string]string{
			"deploy": "-----BEGIN OPENSSH PRIVATE KEY-----",
		},
	}
	_, err := newSSHReadTestClient(t, state).Get(t.Context(), "deploy")
	if err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("Get() error = %v", err)
	}
}

type sshReadOperation struct {
	function string
	method   string
	public   string
}

type sshReadTestState struct {
	mu sync.Mutex

	listData   json.RawMessage
	fetched    map[string]string
	operations []sshReadOperation
}

func newSSHReadTestClient(
	t *testing.T,
	state *sshReadTestState,
) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(state.handle))
	t.Cleanup(server.Close)
	baseClient, err := cpanel.NewClient(server.URL, "test-user", "test-token")
	if err != nil {
		t.Fatalf("create cPanel client: %v", err)
	}

	return NewClient(baseClient)
}

func (state *sshReadTestState) handle(
	writer http.ResponseWriter,
	request *http.Request,
) {
	parameters := request.URL.Query()
	function := parameters.Get("cpanel_jsonapi_func")
	state.mu.Lock()
	state.operations = append(state.operations, sshReadOperation{
		function: function,
		method:   request.Method,
		public:   parameters.Get("pub"),
	})
	state.mu.Unlock()

	if request.Method != http.MethodGet ||
		parameters.Get("cpanel_jsonapi_module") != "SSH" ||
		parameters.Get("cpanel_jsonapi_apiversion") != "2" ||
		parameters.Get("pub") != "1" {
		state.writeEnvelope(
			writer,
			function,
			0,
			[]map[string]any{{"reason": "public GET required"}},
		)
		return
	}

	switch function {
	case operationListKeys:
		state.writeRawSuccess(writer, function, state.listData)
	case operationFetchKey:
		name := parameters.Get("name")
		publicKey, exists := state.fetched[name]
		if !exists {
			state.writeEnvelope(
				writer,
				function,
				0,
				[]map[string]any{{"reason": "key not found"}},
			)
			return
		}
		state.writeEnvelope(
			writer,
			function,
			1,
			[]map[string]any{{
				"key":          publicKey,
				"name":         name + ".pub",
				"future_field": "ignored",
			}},
		)
	default:
		state.writeEnvelope(
			writer,
			function,
			0,
			[]map[string]any{{"reason": "unexpected function"}},
		)
	}
}

func (state *sshReadTestState) writeRawSuccess(
	writer http.ResponseWriter,
	function string,
	data json.RawMessage,
) {
	payload := fmt.Sprintf(
		`{"cpanelresult":{"apiversion":2,"data":%s,"event":{"result":1},"func":%q,"module":"SSH"}}`,
		data,
		function,
	)
	writer.Header().Set("Content-Type", "application/json")
	_, _ = writer.Write([]byte(payload))
}

func (state *sshReadTestState) writeEnvelope(
	writer http.ResponseWriter,
	function string,
	result int,
	data any,
) {
	payload := map[string]any{
		"cpanelresult": map[string]any{
			"apiversion": 2,
			"data":       data,
			"event":      map[string]any{"result": result},
			"func":       function,
			"module":     "SSH",
		},
	}
	writer.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(writer).Encode(payload)
}
