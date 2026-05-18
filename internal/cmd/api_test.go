package cmd

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/ararahq/cli/internal/output"
)

func TestParseAPIHeaders_Empty(t *testing.T) {
	got, err := parseAPIHeaders(nil)
	if err != nil {
		t.Fatalf("parseAPIHeaders(nil): %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestParseAPIHeaders_ValidPairs(t *testing.T) {
	got, err := parseAPIHeaders([]string{
		"X-Trace-Id:abc-123",
		"Idempotency-Key:fixed-key",
		"X-Has-Spaces: with leading space",
	})
	if err != nil {
		t.Fatalf("parseAPIHeaders: %v", err)
	}
	if got["X-Trace-Id"] != "abc-123" {
		t.Errorf("X-Trace-Id: %q", got["X-Trace-Id"])
	}
	if got["Idempotency-Key"] != "fixed-key" {
		t.Errorf("Idempotency-Key: %q", got["Idempotency-Key"])
	}
	if got["X-Has-Spaces"] != "with leading space" {
		t.Errorf("X-Has-Spaces should be trimmed, got %q", got["X-Has-Spaces"])
	}
}

func TestParseAPIHeaders_RejectsMalformed(t *testing.T) {
	cases := []string{
		"no-colon",
		":no-name",
		"no-value:",
		"  :  ",
	}
	for _, header := range cases {
		t.Run(header, func(t *testing.T) {
			if _, err := parseAPIHeaders([]string{header}); err == nil {
				t.Errorf("expected error for %q", header)
			}
		})
	}
}

func TestEnsureLeadingSlash(t *testing.T) {
	cases := map[string]string{
		"/v1/wallet": "/v1/wallet",
		"v1/wallet":  "/v1/wallet",
		"":           "/",
		"/":          "/",
		"v1/x?q=1":   "/v1/x?q=1",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := ensureLeadingSlash(input); got != want {
				t.Errorf("want %q, got %q", want, got)
			}
		})
	}
}

func TestDecodeJSONBody_EmptyReturnsNil(t *testing.T) {
	got, err := decodeJSONBody([]byte(""))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty input, got %v", got)
	}
}

func TestDecodeJSONBody_BlankReturnsNil(t *testing.T) {
	got, err := decodeJSONBody([]byte("   \n\t  "))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for whitespace-only input, got %v", got)
	}
}

func TestDecodeJSONBody_ValidObject(t *testing.T) {
	got, err := decodeJSONBody([]byte(`{"name":"alice","count":3}`))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	asMap, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if asMap["name"] != "alice" {
		t.Errorf("name: %v", asMap["name"])
	}
}

func TestDecodeJSONBody_InvalidJSON(t *testing.T) {
	_, err := decodeJSONBody([]byte("not json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("error should mention JSON, got: %v", err)
	}
}

func TestValidateAPIRequest_RejectsBothBodyAndInput(t *testing.T) {
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	apiBodyFlag = `{"x":1}`
	apiInputFlag = "-"

	if err := validateAPIRequest(); err == nil {
		t.Fatal("expected mutual-exclusion error")
	}
}

func TestValidateAPIRequest_RejectsUnsupportedMethod(t *testing.T) {
	originalMethod := apiMethodFlag
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiMethodFlag = originalMethod
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	apiBodyFlag = ""
	apiInputFlag = ""
	apiMethodFlag = "TRACE"

	err := validateAPIRequest()
	if err == nil {
		t.Fatal("expected error for TRACE method")
	}
	if !strings.Contains(err.Error(), "TRACE") {
		t.Errorf("error should mention method, got: %v", err)
	}
}

func TestValidateAPIRequest_AcceptsLowercaseMethod(t *testing.T) {
	originalMethod := apiMethodFlag
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiMethodFlag = originalMethod
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	apiBodyFlag = ""
	apiInputFlag = ""
	apiMethodFlag = "post"

	if err := validateAPIRequest(); err != nil {
		t.Errorf("lowercase 'post' should normalize to POST, got: %v", err)
	}
}

func TestRenderAPIResponse_WritesIndentedJSON(t *testing.T) {
	buffer := &bytes.Buffer{}
	payload := map[string]any{"hello": "world", "n": 42}

	if err := renderAPIResponse(buffer, payload); err != nil {
		t.Fatalf("render: %v", err)
	}

	rendered := buffer.String()
	if !strings.Contains(rendered, "\"hello\"") || !strings.Contains(rendered, "\"world\"") {
		t.Errorf("output should contain payload, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "  ") {
		t.Errorf("output should be indented, got:\n%s", rendered)
	}
}

func TestRenderAPIResponse_NilIsNoop(t *testing.T) {
	buffer := &bytes.Buffer{}
	if err := renderAPIResponse(buffer, nil); err != nil {
		t.Fatalf("render: %v", err)
	}
	if buffer.Len() != 0 {
		t.Errorf("nil response should produce no output, got %q", buffer.String())
	}
}

func TestRunAPIImpl_GETReturnsBody(t *testing.T) {
	_, client := fakeAPIServerJSON(t, 200, `{"balance":12345}`)
	buffer := &bytes.Buffer{}

	err := runAPIImpl(client, output.FormatText, buffer, "GET", "/v1/wallet/balance", nil, nil, false)
	if err != nil {
		t.Fatalf("runAPIImpl: %v", err)
	}
	if !strings.Contains(buffer.String(), `"balance"`) || !strings.Contains(buffer.String(), "12345") {
		t.Errorf("expected response in output, got: %q", buffer.String())
	}
}

func TestRunAPIImpl_SilentSuppressesOutput(t *testing.T) {
	_, client := fakeAPIServerJSON(t, 200, `{"x":1}`)
	buffer := &bytes.Buffer{}

	if err := runAPIImpl(client, output.FormatText, buffer, "GET", "/v1/x", nil, nil, true); err != nil {
		t.Fatalf("runAPIImpl: %v", err)
	}
	if buffer.Len() != 0 {
		t.Errorf("silent should produce no output, got %q", buffer.String())
	}
}

func TestRunAPIImpl_PostSendsBody(t *testing.T) {
	var (
		mu             sync.Mutex
		receivedMethod string
		receivedPath   string
		receivedBody   []byte
	)

	server, client := fakeAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		receivedBody = body
		mu.Unlock()
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"new"}`))
	})
	_ = server

	buffer := &bytes.Buffer{}
	err := runAPIImpl(client, output.FormatJSON, buffer, "POST", "/v1/segments", map[string]any{"name": "VIPs"}, nil, false)
	if err != nil {
		t.Fatalf("runAPIImpl: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if receivedMethod != "POST" {
		t.Errorf("method: %q", receivedMethod)
	}
	if receivedPath != "/v1/segments" {
		t.Errorf("path: %q", receivedPath)
	}
	if !strings.Contains(string(receivedBody), `"name"`) {
		t.Errorf("body: %q", string(receivedBody))
	}
	if !strings.Contains(buffer.String(), `"new"`) {
		t.Errorf("output should contain response, got: %q", buffer.String())
	}
}

func TestRunAPIImpl_AppendsLeadingSlashIfMissing(t *testing.T) {
	var receivedPath string
	server, client := fakeAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})
	_ = server

	if err := runAPIImpl(client, output.FormatText, &bytes.Buffer{}, "GET", "v1/wallet", nil, nil, true); err != nil {
		t.Fatalf("runAPIImpl: %v", err)
	}
	if receivedPath != "/v1/wallet" {
		t.Errorf("path should be /v1/wallet, got %q", receivedPath)
	}
}

func TestRunAPIImpl_ServerErrorReturnsError(t *testing.T) {
	_, client := fakeAPIServerJSON(t, 500, `{"error":{"code":"server_error","message":"oops"}}`)
	err := runAPIImpl(client, output.FormatText, &bytes.Buffer{}, "GET", "/v1/x", nil, nil, false)
	if err == nil {
		t.Fatal("expected error from 5xx")
	}
}

func TestRunAPIImpl_ForwardsCustomHeaders(t *testing.T) {
	var traceHeader string
	server, client := fakeAPIServer(t, func(w http.ResponseWriter, r *http.Request) {
		traceHeader = r.Header.Get("X-Trace-Id")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	})
	_ = server

	headers := map[string]string{"X-Trace-Id": "trace-abc"}
	if err := runAPIImpl(client, output.FormatText, &bytes.Buffer{}, "GET", "/v1/x", nil, headers, true); err != nil {
		t.Fatalf("runAPIImpl: %v", err)
	}
	if traceHeader != "trace-abc" {
		t.Errorf("custom header not forwarded: %q", traceHeader)
	}
}

func TestRunAPI_WrapperEndToEnd(t *testing.T) {
	originalMethod := apiMethodFlag
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	originalHeader := apiHeaderFlag
	originalSilent := apiSilentFlag
	t.Cleanup(func() {
		apiMethodFlag = originalMethod
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
		apiHeaderFlag = originalHeader
		apiSilentFlag = originalSilent
	})

	apiMethodFlag = "GET"
	apiBodyFlag = ""
	apiInputFlag = ""
	apiHeaderFlag = nil
	apiSilentFlag = true

	_, client := fakeAPIServerJSON(t, 200, `{"ok":true}`)
	withFakeAPIClient(t, client)

	if err := runAPI(nil, []string{"/v1/wallet/balance"}); err != nil {
		t.Errorf("runAPI: %v", err)
	}
}

func TestRunAPI_RejectsInvalidMethod(t *testing.T) {
	originalMethod := apiMethodFlag
	t.Cleanup(func() { apiMethodFlag = originalMethod })
	apiMethodFlag = "TRACE"

	if err := runAPI(nil, []string{"/v1/x"}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestResolveAPIBody_FromFlag(t *testing.T) {
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	apiBodyFlag = `{"key":"value"}`
	apiInputFlag = ""

	body, err := resolveAPIBody()
	if err != nil {
		t.Fatalf("resolveAPIBody: %v", err)
	}
	asMap, ok := body.(map[string]any)
	if !ok || asMap["key"] != "value" {
		t.Errorf("body decoded wrong: %+v", body)
	}
}

func TestResolveAPIBody_FromFile(t *testing.T) {
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	dir := t.TempDir()
	path := dir + "/body.json"
	if err := writeFile(path, `{"file":"yes"}`); err != nil {
		t.Fatal(err)
	}

	apiBodyFlag = ""
	apiInputFlag = path

	body, err := resolveAPIBody()
	if err != nil {
		t.Fatalf("resolveAPIBody: %v", err)
	}
	asMap, ok := body.(map[string]any)
	if !ok || asMap["file"] != "yes" {
		t.Errorf("body from file wrong: %+v", body)
	}
}

func TestResolveAPIBody_NoFlagReturnsNil(t *testing.T) {
	originalBody := apiBodyFlag
	originalInput := apiInputFlag
	t.Cleanup(func() {
		apiBodyFlag = originalBody
		apiInputFlag = originalInput
	})

	apiBodyFlag = ""
	apiInputFlag = ""

	body, err := resolveAPIBody()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body != nil {
		t.Errorf("expected nil, got %v", body)
	}
}

func writeFile(path string, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
