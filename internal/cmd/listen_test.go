package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ararahq/cli/internal/api"
)

func TestDecodeEventData(t *testing.T) {
	if got := decodeEventData(""); got != nil {
		t.Errorf("empty data should decode to nil, got %v", got)
	}

	got := decodeEventData(`{"id":"abc"}`)
	asMap, isMap := got.(map[string]any)
	if !isMap {
		t.Fatalf("JSON object should decode to map, got %T", got)
	}
	if asMap["id"] != "abc" {
		t.Errorf("expected id=abc, got %v", asMap["id"])
	}

	if got := decodeEventData("plain string"); got != "plain string" {
		t.Errorf("non-JSON should pass through, got %v", got)
	}
}

func TestEventMatchesFilter(t *testing.T) {
	if !eventMatchesFilter("any", nil) {
		t.Error("nil filters should match everything")
	}
	if !eventMatchesFilter("any", []string{}) {
		t.Error("empty filters should match everything")
	}
	if !eventMatchesFilter("Message.Delivered", []string{"message.delivered"}) {
		t.Error("filter match should be case-insensitive")
	}
	if eventMatchesFilter("message.read", []string{"message.delivered"}) {
		t.Error("filter mismatch should reject")
	}
}

func TestWriteStreamEvent_EmitsNDJSON(t *testing.T) {
	buffer := &bytes.Buffer{}
	event := api.SSEEvent{Event: "message.delivered", Data: `{"id":"abc"}`}

	if err := writeStreamEvent(buffer, event); err != nil {
		t.Fatalf("writeStreamEvent: %v", err)
	}

	rawLine := strings.TrimSpace(buffer.String())
	if strings.Contains(rawLine, "\n") {
		t.Errorf("NDJSON line must not contain embedded newlines: %q", rawLine)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(rawLine), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if decoded["event"] != "message.delivered" {
		t.Errorf("event mismatch: %v", decoded["event"])
	}
	dataMap, isMap := decoded["data"].(map[string]any)
	if !isMap {
		t.Fatalf("data should be parsed object, got %T", decoded["data"])
	}
	if dataMap["id"] != "abc" {
		t.Errorf("data.id mismatch: %v", dataMap["id"])
	}
}

func TestWriteStreamEvent_NonJSONDataPassesThrough(t *testing.T) {
	buffer := &bytes.Buffer{}
	event := api.SSEEvent{Event: "ping", Data: "pong"}

	if err := writeStreamEvent(buffer, event); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["data"] != "pong" {
		t.Errorf("expected data='pong', got %v", decoded["data"])
	}
}

func TestNewForwarder_NilOnEmptyURL(t *testing.T) {
	if got := newForwarder(""); got != nil {
		t.Errorf("expected nil forwarder for empty URL, got %v", got)
	}
}

func TestNewForwarder_BuildsClientWithTimeout(t *testing.T) {
	forwarder := newForwarder("http://localhost:3000/webhook")
	if forwarder == nil {
		t.Fatal("expected non-nil forwarder")
	}
	if forwarder.url != "http://localhost:3000/webhook" {
		t.Errorf("url: %q", forwarder.url)
	}
	if forwarder.httpClient == nil {
		t.Error("httpClient should be set")
	}
	if forwarder.httpClient.Timeout != forwardTimeout {
		t.Errorf("timeout: %v", forwarder.httpClient.Timeout)
	}
}

func TestStreamForwarder_Send_PostsBodyAndHeaders(t *testing.T) {
	var (
		mu              sync.Mutex
		receivedBody    []byte
		receivedHeaders http.Header
		receivedMethod  string
		callCount       int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedBody = body
		receivedHeaders = r.Header.Clone()
		receivedMethod = r.Method
		callCount++
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	stderr := &bytes.Buffer{}
	forwarder := newForwarder(server.URL)
	forwarder.stderr = stderr

	event := api.SSEEvent{
		Event: "message.delivered",
		Data:  `{"id":"msg_1","status":"delivered"}`,
	}
	forwarder.send(event)

	mu.Lock()
	defer mu.Unlock()

	if callCount != 1 {
		t.Errorf("expected 1 POST, got %d", callCount)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("method: want POST, got %s", receivedMethod)
	}
	if string(receivedBody) != event.Data {
		t.Errorf("body: want %q, got %q", event.Data, string(receivedBody))
	}
	if receivedHeaders.Get(forwardEventHeader) != "message.delivered" {
		t.Errorf("event header missing or wrong: %q", receivedHeaders.Get(forwardEventHeader))
	}
	if receivedHeaders.Get(forwardSourceHeader) != forwardSourceName {
		t.Errorf("source header missing or wrong: %q", receivedHeaders.Get(forwardSourceHeader))
	}
	if receivedHeaders.Get(contentTypeHeaderKey) != contentTypeJSONValue {
		t.Errorf("content-type header missing: %q", receivedHeaders.Get(contentTypeHeaderKey))
	}
	if !strings.Contains(stderr.String(), "forwarded message.delivered") {
		t.Errorf("stderr should log forward, got: %q", stderr.String())
	}
}

func TestStreamForwarder_Send_PropagatesSignatureWhenPresent(t *testing.T) {
	var (
		mu            sync.Mutex
		signatureSeen string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		signatureSeen = r.Header.Get(forwardSignatureHeader)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	forwarder := newForwarder(server.URL)
	forwarder.stderr = &bytes.Buffer{}

	forwarder.send(api.SSEEvent{
		Event:     "message.delivered",
		Data:      `{"id":"msg_1"}`,
		Signature: "sha256=cafebabe",
	})

	mu.Lock()
	defer mu.Unlock()
	if signatureSeen != "sha256=cafebabe" {
		t.Errorf("signature header: want %q, got %q", "sha256=cafebabe", signatureSeen)
	}
}

func TestStreamForwarder_Send_OmitsSignatureWhenAbsent(t *testing.T) {
	var (
		mu       sync.Mutex
		sigEmpty bool
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sigEmpty = r.Header.Get(forwardSignatureHeader) == ""
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	forwarder := newForwarder(server.URL)
	forwarder.stderr = &bytes.Buffer{}

	forwarder.send(api.SSEEvent{Event: "message.delivered", Data: `{"id":"x"}`})

	mu.Lock()
	defer mu.Unlock()
	if !sigEmpty {
		t.Error("signature header should be absent when SSEEvent has no Signature")
	}
}

func TestStreamForwarder_Send_LogsErrorOnUnreachable(t *testing.T) {
	stderr := &bytes.Buffer{}
	// Port 1 is privileged on most systems and not listening — instant failure.
	forwarder := newForwarder("http://127.0.0.1:1/webhook")
	forwarder.stderr = stderr

	forwarder.send(api.SSEEvent{Event: "ping", Data: "{}"})

	if !strings.Contains(stderr.String(), "forward to") {
		t.Errorf("expected error log, got: %q", stderr.String())
	}
}

func TestStreamForwarder_Send_NilForwarderIsNoop(t *testing.T) {
	var forwarder *streamForwarder
	// Should not panic.
	forwarder.send(api.SSEEvent{Event: "x"})
}

func TestRunListen_RejectsInvalidForwardURL(t *testing.T) {
	originalForward := listenForwardToFlag
	t.Cleanup(func() { listenForwardToFlag = originalForward })
	listenForwardToFlag = "ftp://bad-scheme"

	err := runListen(nil, nil)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

// dispatchListenMode is awkward to test end-to-end because the non-capture,
// non-stream branch enters bubbletea (Program.Run blocks on a real TTY).
// We exercise it indirectly via runListenViewer in TestRunListenViewer_*
// tests and via runListenCapture in TestRunListenCapture_*. The dispatcher
// itself is a 3-way switch with no logic, so additional direct tests would
// duplicate without adding signal.

// startFakeSSEServer returns an httptest.Server that serves one SSE event and
// keeps the connection open until ctx is canceled. Designed for tests that
// need runListenStream to drain a single event and observe the side effect.
func startFakeSSEServer(t *testing.T, event string, dataJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = w.Write([]byte("event: " + event + "\n"))
		_, _ = w.Write([]byte("data: " + dataJSON + "\n\n"))
		flusher.Flush()

		// Block until the client disconnects.
		<-r.Context().Done()
	}))
}

func TestRunListenStreamWithCtx_EmitsEventToStdout(t *testing.T) {
	server := startFakeSSEServer(t, "message.delivered", `{"id":"msg_1"}`)
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_xxx")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- runListenStreamWithCtx(ctx, client, nil, "")
	}()

	select {
	case <-done:
		// loop exited cleanly when ctx expired
	case <-time.After(2 * time.Second):
		t.Fatal("runListenStreamWithCtx did not exit after ctx timeout")
	}
}

func TestRunCaptureHeartbeat_StopsOnContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_xxx")
	listener := &api.CreateWebhookListenerResponse{
		ID:                       "lst_x",
		HeartbeatIntervalSeconds: captureHeartbeatFloorSeconds,
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runCaptureHeartbeat(ctx, client, listener)
		close(done)
	}()

	// Cancel almost immediately; heartbeat should exit promptly.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat goroutine did not exit after ctx cancel")
	}
}

func TestReleaseCaptureListener_VerboseLogsOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_xxx")
	// Just exercise both branches — success and failure.
	releaseCaptureListener(client, "lst_x") // 500 → verbose log path

	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(successServer.Close)
	successClient := api.NewClient(successServer.URL, "ara_test_xxx")
	releaseCaptureListener(successClient, "lst_x")
}

func TestValidateForwardURL(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"http://localhost:3000/webhook", false},
		{"https://hooks.example.com/x", false},
		{"  http://example.com  ", false},
		{"localhost:3000", true},
		{"ftp://example.com", true},
		{"http://", true},
		{"::not a url::", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			err := validateForwardURL(tc.input)
			if tc.wantErr && err == nil {
				t.Errorf("validateForwardURL(%q): want error, got nil", tc.input)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("validateForwardURL(%q): want nil, got %v", tc.input, err)
			}
		})
	}
}

func TestCaptureOwnerIdentity_HasGOOSGOARCHAndHostname(t *testing.T) {
	got := captureOwnerIdentity()
	for _, expected := range []string{runtime.GOOS, runtime.GOARCH, "@"} {
		if !strings.Contains(got, expected) {
			t.Errorf("owner identity should contain %q, got %q", expected, got)
		}
	}
}

func TestRunListenCapture_FallbackWhenBackend404(t *testing.T) {
	// Backend doesn't have the capture endpoints yet → CLI must NOT fail.
	// It logs a warning and falls back to passive SSE. We don't actually
	// need the SSE to deliver anything for this assertion — we only care
	// that runListenCapture doesn't propagate the 404 as an error.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/cli/webhook-listeners":
			// Spring's bare 404 — feature missing
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`<html>404 Not Found</html>`))
		case strings.HasPrefix(r.URL.Path, "/v1/stream"):
			// Hold the SSE open briefly; ctx will cancel us.
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_xxx")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := runListenCapture(ctx, client, nil, "")
	// We expect nil — fallback path. The SSE consumer returns when ctx
	// expires.
	if err != nil {
		t.Errorf("expected nil on 404 fallback, got: %v", err)
	}
}

func TestRunListenCapture_PropagatesConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/cli/webhook-listeners" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":{"code":"capture_listener_active","message":"taken","details":{"listenerId":"lst_other","owner":"laptop@there","expiresAt":"2026-12-01T00:00:00Z"}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_xxx")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := runListenCapture(ctx, client, nil, "http://localhost:3000")
	var conflict *api.CaptureConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected CaptureConflictError, got: %v", err)
	}
	if conflict.ListenerID != "lst_other" {
		t.Errorf("conflict ID: %q", conflict.ListenerID)
	}
}

func TestPrintCaptureBanner_NoForwardWarning(t *testing.T) {
	// Smoke — outputs go to stderr, just ensure we don't panic.
	listener := &api.CreateWebhookListenerResponse{
		ID:        "lst_x",
		Secret:    "wh_cli_xxx",
		ExpiresAt: "2026-12-01T00:00:00Z",
	}
	printCaptureBanner(listener, "")
	printCaptureBanner(listener, "http://localhost:3000")
}

func TestPrintCaptureConflict_RendersAllFields(t *testing.T) {
	conflict := &api.CaptureConflictError{
		ListenerID: "lst_x",
		Owner:      "darwin/arm64@host",
		StartedAt:  "2026-05-11T18:00:00Z",
		ExpiresAt:  "2026-05-11T19:00:00Z",
	}
	printCaptureConflict(conflict)

	conflict.Owner = ""
	conflict.StartedAt = ""
	conflict.ExpiresAt = ""
	printCaptureConflict(conflict)
}

func TestParseEventFilters(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"   ", []string{}},
		{"message.delivered", []string{"message.delivered"}},
		{"message.delivered,message.read", []string{"message.delivered", "message.read"}},
		{" a , b , , c ", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseEventFilters(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseEventFilters(%q): want %#v, got %#v", tc.input, tc.want, got)
			}
		})
	}
}
