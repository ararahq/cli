package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type capturedRequest struct {
	method string
	path   string
	header http.Header
	body   string
}

type recordingServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

func newRecordingServer(t *testing.T, handler func(attempt int, w http.ResponseWriter, r *http.Request)) *recordingServer {
	t.Helper()
	rs := &recordingServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		rs.mu.Lock()
		attempt := len(rs.requests)
		rs.requests = append(rs.requests, capturedRequest{
			method: r.Method,
			path:   r.URL.RequestURI(),
			header: r.Header.Clone(),
			body:   string(bodyBytes),
		})
		rs.mu.Unlock()
		handler(attempt, w, r)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *recordingServer) snapshot() []capturedRequest {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	out := make([]capturedRequest, len(rs.requests))
	copy(out, rs.requests)
	return out
}

func newTestClient(baseURL string) *Client {
	client := NewClient(baseURL, "ara_test_abcdef1234567890")
	client.sleep = func(time.Duration) {}
	client.randFloat = func() float64 { return 0 }
	keyCounter := uint64(0)
	client.newIdempotencyKey = func() string {
		next := atomic.AddUint64(&keyCounter, 1)
		return fmt.Sprintf("test-key-%d", next)
	}
	client.backoffBase = time.Millisecond
	client.backoffCap = 5 * time.Millisecond
	return client
}

func TestClient_Get_Success(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc","status":"ok"}`))
	})

	client := newTestClient(server.URL)

	var result struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := client.Get("/v1/foo", &result); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}

	if result.ID != "abc" || result.Status != "ok" {
		t.Errorf("unexpected result: %+v", result)
	}

	requests := server.snapshot()
	if len(requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(requests))
	}

	authHeader := requests[0].header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ara_test_") {
		t.Errorf("missing/invalid Authorization header: %q", authHeader)
	}
	if !strings.HasPrefix(requests[0].header.Get("User-Agent"), "arara-cli/") {
		t.Errorf("unexpected User-Agent: %q", requests[0].header.Get("User-Agent"))
	}
	if requests[0].header.Get("Idempotency-Key") != "" {
		t.Errorf("GET should not carry Idempotency-Key, got %q", requests[0].header.Get("Idempotency-Key"))
	}
}

func TestClient_Post_AutoGeneratesIdempotencyKey(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	})

	client := newTestClient(server.URL)

	var result struct {
		ID string `json:"id"`
	}
	if err := client.Post("/v1/things", map[string]string{"name": "y"}, &result); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}

	requests := server.snapshot()
	if got := requests[0].header.Get("Idempotency-Key"); got != "test-key-1" {
		t.Errorf("expected auto-generated Idempotency-Key=test-key-1, got %q", got)
	}
	if requests[0].body != `{"name":"y"}` {
		t.Errorf("unexpected request body: %q", requests[0].body)
	}
}

func TestClient_Post_RetryReusesIdempotencyKey(t *testing.T) {
	server := newRecordingServer(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"code":"server","message":"boom"}}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	})

	client := newTestClient(server.URL)

	var result struct {
		ID string `json:"id"`
	}
	if err := client.Post("/v1/things", map[string]string{"a": "b"}, &result); err != nil {
		t.Fatalf("Post returned error: %v", err)
	}

	requests := server.snapshot()
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests after retries, got %d", len(requests))
	}

	keys := map[string]struct{}{}
	for _, req := range requests {
		keys[req.header.Get("Idempotency-Key")] = struct{}{}
	}
	if len(keys) != 1 {
		t.Errorf("expected same Idempotency-Key across retries, got %d distinct", len(keys))
	}
}

func TestClient_DoWithHeaders_PreservesCustomIdempotencyKey(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	client := newTestClient(server.URL)

	headers := map[string]string{"Idempotency-Key": "user-supplied-42"}
	if err := client.DoWithHeaders(http.MethodPost, "/v1/things", map[string]string{}, nil, headers); err != nil {
		t.Fatalf("DoWithHeaders returned error: %v", err)
	}

	requests := server.snapshot()
	if got := requests[0].header.Get("Idempotency-Key"); got != "user-supplied-42" {
		t.Errorf("expected Idempotency-Key=user-supplied-42 (caller value), got %q", got)
	}
}

func TestClient_DoWithHeaders_EmptyCustomKeyFallsThroughToAutoGen(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := newTestClient(server.URL)

	headers := map[string]string{"Idempotency-Key": ""}
	if err := client.DoWithHeaders(http.MethodPost, "/v1/things", nil, nil, headers); err != nil {
		t.Fatalf("DoWithHeaders returned error: %v", err)
	}

	requests := server.snapshot()
	if got := requests[0].header.Get("Idempotency-Key"); got != "test-key-1" {
		t.Errorf("expected auto-generated Idempotency-Key when custom is empty, got %q", got)
	}
}

func TestClient_Retry_On5xx(t *testing.T) {
	server := newRecordingServer(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt < 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	client := newTestClient(server.URL)

	if err := client.Get("/v1/foo", nil); err != nil {
		t.Fatalf("expected eventual success, got: %v", err)
	}

	if got := len(server.snapshot()); got != 2 {
		t.Errorf("expected 2 attempts (1 fail + 1 success), got %d", got)
	}
}

func TestClient_NoRetry_On4xx(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"bad","message":"nope"}}`))
	})

	client := newTestClient(server.URL)

	err := client.Get("/v1/foo", nil)
	if err == nil {
		t.Fatal("expected error on 4xx, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", apiErr.StatusCode)
	}

	if got := len(server.snapshot()); got != 1 {
		t.Errorf("4xx must not retry, got %d attempts", got)
	}
}

func TestClient_Retry_On429HonorsRetryAfter(t *testing.T) {
	var observedSleeps []time.Duration
	var sleepMu sync.Mutex

	server := newRecordingServer(t, func(attempt int, w http.ResponseWriter, _ *http.Request) {
		if attempt < 1 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	client := newTestClient(server.URL)
	client.sleep = func(duration time.Duration) {
		sleepMu.Lock()
		observedSleeps = append(observedSleeps, duration)
		sleepMu.Unlock()
	}

	if err := client.Get("/v1/foo", nil); err != nil {
		t.Fatalf("expected success after 429, got: %v", err)
	}

	if len(observedSleeps) != 1 {
		t.Fatalf("expected 1 sleep before retry, got %d", len(observedSleeps))
	}
	if observedSleeps[0] != 2*time.Second {
		t.Errorf("expected 2s sleep from Retry-After header, got %v", observedSleeps[0])
	}
}

func TestClient_RetryExhausts(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"code":"down","message":"upstream"}}`))
	})

	client := newTestClient(server.URL)

	err := client.Get("/v1/foo", nil)
	if err == nil {
		t.Fatal("expected exhausted retry error, got nil")
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("error should mention attempt count, got: %v", err)
	}
	if got := len(server.snapshot()); got != 3 {
		t.Errorf("expected 3 attempts, got %d", got)
	}
}

func TestClient_NoRetry_OnContextCanceled(t *testing.T) {
	calls := atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()
	server.Close()

	client := newTestClient(server.URL)
	client.httpClient = &http.Client{
		Transport: &cancelTransport{cause: context.Canceled},
	}

	err := client.Get("/v1/foo", nil)
	if err == nil {
		t.Fatal("expected error on context canceled, got nil")
	}
	if calls.Load() != 0 {
		t.Errorf("transport should not have been called via real network, got %d", calls.Load())
	}
}

type cancelTransport struct {
	cause error
}

func (c *cancelTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, c.cause
}

func TestClient_Mode(t *testing.T) {
	if NewClient("https://x", "ara_live_xxx").Mode() != "LIVE" {
		t.Error("ara_live_ key should yield LIVE mode")
	}
	if NewClient("https://x", "ara_test_xxx").Mode() != "TEST" {
		t.Error("ara_test_ key should yield TEST mode")
	}
}

func TestClient_BaseURL_TrimsTrailingSlash(t *testing.T) {
	client := NewClient("https://api.example.com/", "ara_test_x")
	if client.BaseURL() != "https://api.example.com" {
		t.Errorf("BaseURL should strip trailing slash, got %q", client.BaseURL())
	}
}

func TestComputeBackoff_RespectsRetryAfter(t *testing.T) {
	client := newTestClient("http://x")
	got := client.computeBackoff(0, 3*time.Second)
	if got != 3*time.Second {
		t.Errorf("Retry-After should win over jitter, got %v", got)
	}
}

func TestComputeBackoff_CapsRetryAfter(t *testing.T) {
	client := newTestClient("http://x")
	got := client.computeBackoff(0, 5*time.Minute)
	if got != maxRetryAfter {
		t.Errorf("Retry-After should be capped at %v, got %v", maxRetryAfter, got)
	}
}

func TestComputeBackoff_ExponentialWithJitter(t *testing.T) {
	client := newTestClient("http://x")
	client.backoffBase = 100 * time.Millisecond
	client.backoffCap = time.Second
	client.randFloat = func() float64 { return 1 }

	gotZero := client.computeBackoff(0, 0)
	gotOne := client.computeBackoff(1, 0)
	gotHigh := client.computeBackoff(10, 0)

	if gotZero != 100*time.Millisecond {
		t.Errorf("attempt 0 with jitter=1 should equal base, got %v", gotZero)
	}
	if gotOne != 200*time.Millisecond {
		t.Errorf("attempt 1 with jitter=1 should equal 2*base, got %v", gotOne)
	}
	if gotHigh != time.Second {
		t.Errorf("attempt 10 should be capped at backoffCap, got %v", gotHigh)
	}

	client.randFloat = func() float64 { return 0 }
	if got := client.computeBackoff(2, 0); got != 0 {
		t.Errorf("jitter=0 should yield zero delay, got %v", got)
	}
}

func TestComputeBackoff_ClampsRandFloat(t *testing.T) {
	client := newTestClient("http://x")
	client.backoffBase = 100 * time.Millisecond
	client.backoffCap = time.Second
	client.randFloat = func() float64 { return -0.5 }
	if got := client.computeBackoff(0, 0); got != 0 {
		t.Errorf("negative jitter should clamp to 0, got %v", got)
	}
	client.randFloat = func() float64 { return 5 }
	if got := client.computeBackoff(0, 0); got != 100*time.Millisecond {
		t.Errorf("jitter > 1 should clamp to base, got %v", got)
	}
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"", 0},
		{"   ", 0},
		{"0", 0},
		{"-3", 0},
		{"5", 5 * time.Second},
		{"99999", maxRetryAfter},
		{"garbage", 0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := parseRetryAfter(tc.input); got != tc.want {
				t.Errorf("parseRetryAfter(%q): want %v, got %v", tc.input, tc.want, got)
			}
		})
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	future := time.Now().Add(3 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	if got <= 0 || got > maxRetryAfter {
		t.Errorf("expected positive bounded duration from HTTP date, got %v", got)
	}

	past := time.Now().Add(-30 * time.Second).UTC().Format(http.TimeFormat)
	if pastGot := parseRetryAfter(past); pastGot != 0 {
		t.Errorf("expected 0 for past date, got %v", pastGot)
	}

	veryFuture := time.Now().Add(2 * time.Hour).UTC().Format(http.TimeFormat)
	if vGot := parseRetryAfter(veryFuture); vGot != maxRetryAfter {
		t.Errorf("expected cap at maxRetryAfter, got %v", vGot)
	}
}

func TestClassifyForRetry(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantRetry bool
		wantAfter time.Duration
	}{
		{"context canceled", context.Canceled, false, 0},
		{"4xx api error", &APIError{StatusCode: 404}, false, 0},
		{"401 not retryable", &APIError{StatusCode: 401}, false, 0},
		{"429 retryable with retry-after", &APIError{StatusCode: 429, RetryAfter: 2 * time.Second}, true, 2 * time.Second},
		{"500 retryable", &APIError{StatusCode: 500}, true, 0},
		{"503 retryable", &APIError{StatusCode: 503}, true, 0},
		{"transport error retryable", errors.New("boom"), true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotRetry, gotAfter := classifyForRetry(tc.err)
			if gotRetry != tc.wantRetry || gotAfter != tc.wantAfter {
				t.Errorf("classify(%v): want (%v,%v), got (%v,%v)", tc.err, tc.wantRetry, tc.wantAfter, gotRetry, gotAfter)
			}
		})
	}
}

func TestBuildRequestHeaders(t *testing.T) {
	idGen := func() string { return "generated" }

	t.Run("safe method skips idempotency", func(t *testing.T) {
		got := buildRequestHeaders(http.MethodGet, nil, idGen)
		if _, exists := got["Idempotency-Key"]; exists {
			t.Error("GET must not get Idempotency-Key")
		}
	})

	t.Run("unsafe method gets auto-generated", func(t *testing.T) {
		got := buildRequestHeaders(http.MethodPost, nil, idGen)
		if got["Idempotency-Key"] != "generated" {
			t.Errorf("want generated key, got %v", got)
		}
	})

	t.Run("custom non-empty wins", func(t *testing.T) {
		got := buildRequestHeaders(http.MethodPost, map[string]string{"Idempotency-Key": "custom"}, idGen)
		if got["Idempotency-Key"] != "custom" {
			t.Errorf("want custom key preserved, got %v", got)
		}
	})

	t.Run("custom empty falls through to auto-gen", func(t *testing.T) {
		got := buildRequestHeaders(http.MethodPost, map[string]string{"Idempotency-Key": ""}, idGen)
		if got["Idempotency-Key"] != "generated" {
			t.Errorf("want generated key when custom empty, got %v", got)
		}
	})

	t.Run("DELETE also gets idempotency", func(t *testing.T) {
		got := buildRequestHeaders(http.MethodDelete, nil, idGen)
		if got["Idempotency-Key"] != "generated" {
			t.Errorf("DELETE should get key, got %v", got)
		}
	})
}

func TestSerializeBody(t *testing.T) {
	got, err := serializeBody(nil, false)
	if err != nil || got != nil {
		t.Errorf("nil body should return (nil,nil), got (%v,%v)", got, err)
	}

	got, err = serializeBody(map[string]int{"a": 1}, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("want {\"a\":1}, got %s", got)
	}

	if _, err := serializeBody(make(chan int), false); err == nil {
		t.Error("expected error serializing non-marshallable value")
	}
}

func TestDecodeResponse(t *testing.T) {
	if err := decodeResponse(nil, nil); err != nil {
		t.Errorf("nil result+body should be nop, got %v", err)
	}
	if err := decodeResponse([]byte(`{}`), nil); err != nil {
		t.Errorf("nil result should be nop, got %v", err)
	}

	var into struct {
		A int `json:"a"`
	}
	if err := decodeResponse([]byte(`{"a":7}`), &into); err != nil {
		t.Fatal(err)
	}
	if into.A != 7 {
		t.Errorf("want 7, got %d", into.A)
	}

	if err := decodeResponse([]byte(`not json`), &into); err == nil {
		t.Error("expected JSON parse error")
	}
}

func TestClient_Delete_NoBody_NoResult(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	client := newTestClient(server.URL)

	if err := client.Delete("/v1/things/123"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	requests := server.snapshot()
	if requests[0].method != http.MethodDelete {
		t.Errorf("want DELETE, got %s", requests[0].method)
	}
	if requests[0].header.Get("Idempotency-Key") == "" {
		t.Error("DELETE should carry idempotency key")
	}
}

func TestClient_BadResponseBody(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("nope"))
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort"))
		_ = conn.Close()
	})

	client := newTestClient(server.URL)
	client.httpClient.Timeout = 200 * time.Millisecond

	var into map[string]any
	err := client.Get("/v1/foo", &into)
	if err == nil {
		t.Fatal("expected read error from truncated body, got nil")
	}
}

func TestClient_VerboseDoesNotPanic(t *testing.T) {
	server := newRecordingServer(t, func(_ int, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	client := newTestClient(server.URL)
	client.SetVerbose(true)

	if err := client.Post("/v1/things", map[string]string{"a": "b"}, nil); err != nil {
		t.Fatalf("verbose Post failed: %v", err)
	}
}

func TestBuildUserAgent(t *testing.T) {
	got := buildUserAgent()
	if !strings.HasPrefix(got, "arara-cli/") {
		t.Errorf("user agent must start with arara-cli/, got %q", got)
	}
}

func TestClient_BadURL(t *testing.T) {
	client := newTestClient("http://[::1")
	err := client.Get("/foo", nil)
	if err == nil {
		t.Fatal("expected error from malformed base URL, got nil")
	}
}
