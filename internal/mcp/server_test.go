package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"

	"github.com/ararahq/cli/internal/api"
)

func newFakeAPIClient(t *testing.T, handler http.HandlerFunc) *api.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
}

func newServerForTest(t *testing.T, apiClient *api.Client, allowWriteTools bool) *Server {
	t.Helper()
	server, err := NewServer(apiClient, Options{
		AllowWriteTools: allowWriteTools,
		Logger:          discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return server
}

func extractText(t *testing.T, result *mcpsdk.CallToolResult) string {
	t.Helper()
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	textContent, isText := result.Content[0].(mcpsdk.TextContent)
	if !isText {
		t.Fatalf("first content is not text, got %T", result.Content[0])
	}
	return textContent.Text
}

func unmarshalEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decode envelope: %v (raw=%q)", err, raw)
	}
	return decoded
}

func callRequest(name string, args map[string]any) mcpsdk.CallToolRequest {
	return mcpsdk.CallToolRequest{
		Params: mcpsdk.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func TestNewServer_RejectsNilClient(t *testing.T) {
	if _, err := NewServer(nil, Options{}); !errors.Is(err, ErrNoAPIClient) {
		t.Errorf("want ErrNoAPIClient, got: %v", err)
	}
}

func TestNewServer_RegistersReadToolsByDefault(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)

	tools := server.ListToolNames()
	if len(tools) != 10 {
		t.Errorf("expected 10 read tools, got %d: %v", len(tools), tools)
	}
	for _, name := range tools {
		if !strings.HasPrefix(name, "arara_") {
			t.Errorf("all tools must be prefixed with arara_, got %q", name)
		}
	}
}

func TestNewServer_RegistersWriteToolsWhenAllowed(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	tools := server.ListToolNames()
	if len(tools) != 13 {
		t.Errorf("expected 13 tools with writes, got %d", len(tools))
	}

	expectedWriteTools := []string{toolSendMessage, toolCreateCampaign, toolImportContacts}
	for _, name := range expectedWriteTools {
		if !slicesContains(tools, name) {
			t.Errorf("missing write tool %q in %v", name, tools)
		}
	}
}

func TestHandleListTemplates_Success(t *testing.T) {
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t1","name":"hello","language":"pt_BR","body":"oi"}]`))
	})
	server := newServerForTest(t, apiClient, false)

	result, err := server.handleListTemplates(context.Background(), callRequest(toolListTemplates, nil))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error result: %s", extractText(t, result))
	}
	envelope := unmarshalEnvelope(t, extractText(t, result))
	if envelope["ok"] != true {
		t.Errorf("expected ok=true, got %v", envelope["ok"])
	}
}

func TestHandleGetMessageStatus_RequiresArgument(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)

	result, err := server.handleGetMessageStatus(context.Background(), callRequest(toolGetMessageStatus, map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Errorf("expected error result on missing argument")
	}
	envelope := unmarshalEnvelope(t, extractText(t, result))
	errorBody, ok := envelope["error"].(map[string]any)
	if !ok || errorBody["code"] != "ararahq-invalid-argument" {
		t.Errorf("expected ararahq-invalid-argument code, got %v", envelope)
	}
}

func TestHandleGetMessageStatus_PropagatesAPIError(t *testing.T) {
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"missing"}}`))
	})
	server := newServerForTest(t, apiClient, false)

	result, err := server.handleGetMessageStatus(context.Background(),
		callRequest(toolGetMessageStatus, map[string]any{argMessageID: "m1"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result")
	}
	envelope := unmarshalEnvelope(t, extractText(t, result))
	if envelope["statusCode"].(float64) != 404 {
		t.Errorf("expected statusCode=404, got %v", envelope["statusCode"])
	}
}

func TestHandleListContacts_DefaultsAndClamps(t *testing.T) {
	captured := struct {
		path string
	}{}
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"contacts":[],"total":0}`))
	})
	server := newServerForTest(t, apiClient, false)

	if _, err := server.handleListContacts(context.Background(),
		callRequest(toolListContacts, map[string]any{argPage: float64(-3), argSize: float64(1000)})); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(captured.path, "page=0") {
		t.Errorf("page=-3 should clamp to 0, got %q", captured.path)
	}
	if !strings.Contains(captured.path, "size=100") {
		t.Errorf("size=1000 should clamp to 100, got %q", captured.path)
	}
}

func TestHandleEstimateCampaign_RejectsZeroRecipients(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)

	result, err := server.handleEstimateCampaign(context.Background(),
		callRequest(toolEstimateCampaign, map[string]any{
			argTemplateName:   "hello",
			argRecipientCount: float64(0),
		}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error on zero recipients")
	}
}

func TestHandleSendMessage_DryRunSkipsAPICall(t *testing.T) {
	callCount := 0
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if strings.HasPrefix(r.URL.Path, "/v1/campaigns/estimate") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"totalCost":1.5,"templateCategory":"UTILITY","unitPrice":1.5,"recipientCount":1}`))
			return
		}
		t.Errorf("unexpected request to %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	})

	server := newServerForTest(t, apiClient, true)

	result, err := server.handleSendMessage(context.Background(),
		callRequest(toolSendMessage, map[string]any{
			argTo:           "+5511",
			argTemplateName: "hello",
			argDryRun:       true,
		}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("dry-run should succeed, got: %s", extractText(t, result))
	}
	envelope := unmarshalEnvelope(t, extractText(t, result))
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %v", envelope)
	}
	if data["method"] != "POST" || data["path"] != "/v1/messages" {
		t.Errorf("dry-run preview missing method/path, got %v", data)
	}
}

func TestHandleSendMessage_RejectsBothTemplateAndBody(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleSendMessage(context.Background(),
		callRequest(toolSendMessage, map[string]any{
			argTo:           "+5511",
			argTemplateName: "hello",
			argBody:         "hi",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error on both template and body")
	}
}

func TestHandleSendMessage_RejectsNeitherTemplateNorBody(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleSendMessage(context.Background(),
		callRequest(toolSendMessage, map[string]any{argTo: "+5511"}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error when neither template nor body provided")
	}
}

func TestHandleSendMessage_HonorsCustomIdempotencyKey(t *testing.T) {
	captured := struct {
		key string
	}{}
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured.key = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"m1","status":"queued"}`))
	})
	server := newServerForTest(t, apiClient, true)

	_, err := server.handleSendMessage(context.Background(),
		callRequest(toolSendMessage, map[string]any{
			argTo:             "+5511",
			argBody:           "hi",
			argIdempotencyKey: "user-supplied-42",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if captured.key != "user-supplied-42" {
		t.Errorf("expected custom Idempotency-Key passed through, got %q", captured.key)
	}
}

func TestHandleCreateCampaign_RequiresContacts(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleCreateCampaign(context.Background(),
		callRequest(toolCreateCampaign, map[string]any{
			argCampaignName: "promo",
			argTemplateName: "hello",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error on missing contacts")
	}
}

func TestHandleCreateCampaign_ParsesContacts(t *testing.T) {
	captured := struct {
		body string
	}{}
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		bodyBytes := make([]byte, 4096)
		n, _ := r.Body.Read(bodyBytes)
		captured.body = string(bodyBytes[:n])
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"c1","status":"queued","totalMessages":1}`))
	})
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleCreateCampaign(context.Background(),
		callRequest(toolCreateCampaign, map[string]any{
			argCampaignName: "promo",
			argTemplateName: "hello",
			argContacts: []any{
				map[string]any{argTo: "+5511", argVariables: []any{"John"}},
			},
		}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got: %s", extractText(t, result))
	}
	if !strings.Contains(captured.body, "+5511") {
		t.Errorf("contact payload not propagated, got %q", captured.body)
	}
}

func TestHandleImportContacts_ParsesContacts(t *testing.T) {
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"importId":"imp_1"}`))
	})
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleImportContacts(context.Background(),
		callRequest(toolImportContacts, map[string]any{
			argContacts: []any{
				map[string]any{"phone": "+5511", "name": "John", "email": "x@y.com"},
			},
		}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got: %s", extractText(t, result))
	}
}

func TestHandleImportContacts_RejectsBadShape(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	result, err := server.handleImportContacts(context.Background(),
		callRequest(toolImportContacts, map[string]any{
			argContacts: []any{
				map[string]any{"name": "John"}, // missing phone
			},
		}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error on contact missing phone")
	}
}

func TestClassifyError_APIErrorWithRetryAfter(t *testing.T) {
	apiErr := &api.APIError{
		StatusCode: 429,
		Code:       "ararahq-rate-limit",
		Message:    "slow down",
		RetryAfter: 2_000_000_000, // 2s
	}
	body := classifyError(apiErr)
	if body.Code != "ararahq-rate-limit" {
		t.Errorf("code lost: %v", body)
	}
	if !body.Retryable {
		t.Error("429 should be retryable")
	}
	if body.RetryAfter != 2000 {
		t.Errorf("retryAfter should convert to ms, got %d", body.RetryAfter)
	}
}

func TestClassifyError_4xxNotRetryable(t *testing.T) {
	body := classifyError(&api.APIError{StatusCode: 400, Message: "bad"})
	if body.Retryable {
		t.Error("400 should not be retryable")
	}
}

func TestClassifyError_PlainError(t *testing.T) {
	body := classifyError(errors.New("boom"))
	if body.Message != "boom" {
		t.Errorf("message lost: %v", body)
	}
	if !body.Retryable {
		t.Error("plain transport errors default to retryable")
	}
}

func TestClassifyError_Nil(t *testing.T) {
	body := classifyError(nil)
	if body.Message != "unknown error" {
		t.Errorf("nil error should yield 'unknown error' sentinel, got %q", body.Message)
	}
}

func TestSuccessResult_EncodesPayload(t *testing.T) {
	result, err := successResult(map[string]string{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Error("success result should not be marked as error")
	}
	text := extractText(t, result)
	if !strings.Contains(text, "\"k\": \"v\"") {
		t.Errorf("payload not in result, got %q", text)
	}
}

func TestStringSliceFromAny(t *testing.T) {
	if got := stringSliceFromAny([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Errorf("[]string passthrough failed: %v", got)
	}
	if got := stringSliceFromAny([]any{"a", 1, "b"}); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("[]any filter failed: %v", got)
	}
	if got := stringSliceFromAny(42); got != nil {
		t.Errorf("unsupported type should yield nil, got %v", got)
	}
}

func TestOptionalHelpers(t *testing.T) {
	req := callRequest("x", map[string]any{
		"s": "hello",
		"i": float64(7),
		"b": true,
	})

	if got := optionalString(req, "s", "fallback"); got != "hello" {
		t.Errorf("optionalString: want hello, got %q", got)
	}
	if got := optionalString(req, "missing", "fallback"); got != "fallback" {
		t.Errorf("optionalString missing: want fallback, got %q", got)
	}
	if got := optionalInt(req, "i", -1); got != 7 {
		t.Errorf("optionalInt: want 7, got %d", got)
	}
	if got := optionalInt(req, "missing", -1); got != -1 {
		t.Errorf("optionalInt fallback: want -1, got %d", got)
	}
	if got := optionalBool(req, "b", false); !got {
		t.Error("optionalBool: want true")
	}
	if got := optionalBool(req, "missing", false); got {
		t.Error("optionalBool fallback should be false")
	}
}

func TestAllReadHandlers_HappyPath(t *testing.T) {
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/v1/templates/t1/status"):
			_, _ = w.Write([]byte(`{"status":"APPROVED","category":"MARKETING"}`))
		case strings.HasPrefix(r.URL.Path, "/v1/contacts/stats"):
			_, _ = w.Write([]byte(`{"total":42}`))
		case strings.HasPrefix(r.URL.Path, "/v1/contacts/"):
			_, _ = w.Write([]byte(`{"id":"c1","phone":"+5511"}`))
		case strings.HasPrefix(r.URL.Path, "/organizations/me/numbers"):
			_, _ = w.Write([]byte(`[{"id":"n1","phoneNumber":"+5511"}]`))
		case strings.HasPrefix(r.URL.Path, "/dashboard/metrics"):
			_, _ = w.Write([]byte(`{"sent":10}`))
		case strings.HasPrefix(r.URL.Path, "/dashboard/wallet/balance"):
			_, _ = w.Write([]byte(`{"balance":500}`))
		case strings.HasPrefix(r.URL.Path, "/v1/campaigns/estimate"):
			_, _ = w.Write([]byte(`{"totalCost":1.5,"unitPrice":1.5,"recipientCount":5,"templateCategory":"UTILITY"}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := newServerForTest(t, apiClient, false)

	checks := []struct {
		name    string
		handler func(context.Context, mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error)
		args    map[string]any
	}{
		{toolGetTemplateStatus, server.handleGetTemplateStatus, map[string]any{argTemplateID: "t1"}},
		{toolGetContact, server.handleGetContact, map[string]any{argPhone: "+5511"}},
		{toolGetContactStats, server.handleGetContactStats, nil},
		{toolGetMetrics, server.handleGetMetrics, map[string]any{argMode: "live"}},
		{toolGetWalletBalance, server.handleGetWalletBalance, map[string]any{argMode: "live"}},
		{toolListNumbers, server.handleListNumbers, nil},
		{toolEstimateCampaign, server.handleEstimateCampaign, map[string]any{
			argTemplateName:   "hello",
			argRecipientCount: float64(5),
		}},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			result, err := check.handler(context.Background(), callRequest(check.name, check.args))
			if err != nil {
				t.Fatalf("%s: server error %v", check.name, err)
			}
			if result.IsError {
				t.Errorf("%s: unexpected error result: %s", check.name, extractText(t, result))
			}
		})
	}
}

func TestRequiredArgValidation_AllHandlers(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, true)

	checks := []struct {
		name    string
		handler func(context.Context, mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error)
	}{
		{toolGetTemplateStatus, server.handleGetTemplateStatus},
		{toolGetContact, server.handleGetContact},
		{toolEstimateCampaign, server.handleEstimateCampaign},
		{toolSendMessage, server.handleSendMessage},
		{toolCreateCampaign, server.handleCreateCampaign},
	}

	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			result, err := check.handler(context.Background(), callRequest(check.name, map[string]any{}))
			if err != nil {
				t.Fatal(err)
			}
			if !result.IsError {
				t.Errorf("%s: expected error on empty args", check.name)
			}
		})
	}
}

func TestParseCampaignContacts_ErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing contacts", map[string]any{}},
		{"wrong shape", map[string]any{argContacts: "not-an-array"}},
		{"entry not object", map[string]any{argContacts: []any{"string-not-object"}}},
		{"entry missing to", map[string]any{argContacts: []any{map[string]any{argVariables: []any{"x"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseCampaignContacts(callRequest(toolCreateCampaign, tc.args)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseContactRequests_ErrorPaths(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"missing contacts", map[string]any{}},
		{"wrong shape", map[string]any{argContacts: 42}},
		{"entry not object", map[string]any{argContacts: []any{1}}},
		{"entry missing phone", map[string]any{argContacts: []any{map[string]any{"name": "John"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseContactRequests(callRequest(toolImportContacts, tc.args)); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestParseContactRequests_AllOptionalFields(t *testing.T) {
	req := callRequest(toolImportContacts, map[string]any{
		argContacts: []any{
			map[string]any{
				"phone":      "+5511",
				"name":       "John",
				"email":      "x@y.com",
				"attributes": map[string]any{"tier": "gold"},
			},
		},
	})
	contacts, err := parseContactRequests(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts))
	}
	if contacts[0].Name != "John" || contacts[0].Email != "x@y.com" {
		t.Errorf("optional fields lost: %+v", contacts[0])
	}
	if contacts[0].Attributes["tier"] != "gold" {
		t.Errorf("attributes lost: %+v", contacts[0].Attributes)
	}
}

func TestInstrumentHandler_LogsToolError(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)

	handler := server.instrumentHandler("tool-x", func(_ context.Context, _ mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		result := mcpsdk.NewToolResultError("boom")
		return result, nil
	})
	result, err := handler(context.Background(), callRequest("tool-x", nil))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error result to propagate")
	}
}

func TestInstrumentHandler_LogsServerError(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)

	sentinelErr := errors.New("transport down")
	handler := server.instrumentHandler("tool-x", func(_ context.Context, _ mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return nil, sentinelErr
	})
	_, err := handler(context.Background(), callRequest("tool-x", nil))
	if !errors.Is(err, sentinelErr) {
		t.Errorf("server-level error should pass through, got: %v", err)
	}
}

func TestSendMessage_NoIdempotencyKey_AutoGenerates(t *testing.T) {
	captured := struct {
		key string
	}{}
	apiClient := newFakeAPIClient(t, func(w http.ResponseWriter, r *http.Request) {
		captured.key = r.Header.Get("Idempotency-Key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"m1","status":"queued"}`))
	})
	server := newServerForTest(t, apiClient, true)

	_, err := server.handleSendMessage(context.Background(),
		callRequest(toolSendMessage, map[string]any{
			argTo:   "+5511",
			argBody: "hi",
		}))
	if err != nil {
		t.Fatal(err)
	}
	if captured.key == "" {
		t.Error("Idempotency-Key should be auto-generated when not supplied")
	}
}

func TestMCPServer_Exposed(t *testing.T) {
	apiClient := api.NewClient("http://nowhere", "ara_test_xxxxxxxxxxxx")
	server := newServerForTest(t, apiClient, false)
	if server.MCPServer() == nil {
		t.Error("MCPServer() should return the underlying server")
	}
}

func slicesContains(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}
