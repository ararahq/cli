package telemetry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHTTPSender_Send_GoldenPath(t *testing.T) {
	var (
		mu             sync.Mutex
		receivedPath   string
		receivedMethod string
		receivedCT     string
		receivedBody   []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		receivedPath = r.URL.Path
		receivedMethod = r.Method
		receivedCT = r.Header.Get("Content-Type")
		receivedBody = body
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	sender := NewHTTPSender(server.URL)
	events := []Event{
		{
			Timestamp:   time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			Command:     "send",
			ExitCode:    0,
			DurationMs:  150,
			CLIVersion:  "0.2.0",
			Platform:    "darwin/arm64",
			AnonymousID: "anon-1",
		},
	}

	if err := sender.Send(events); err != nil {
		t.Fatalf("Send: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if receivedPath != DefaultIngestPath {
		t.Errorf("path: want %q, got %q", DefaultIngestPath, receivedPath)
	}
	if receivedMethod != http.MethodPost {
		t.Errorf("method: want POST, got %s", receivedMethod)
	}
	if receivedCT != contentTypeJSON {
		t.Errorf("content-type: want %q, got %q", contentTypeJSON, receivedCT)
	}

	var decoded httpSendBody
	if err := json.Unmarshal(receivedBody, &decoded); err != nil {
		t.Fatalf("server received malformed JSON: %v", err)
	}
	if len(decoded.Events) != 1 || decoded.Events[0].Command != "send" {
		t.Errorf("server received wrong events: %+v", decoded.Events)
	}
}

func TestHTTPSender_Send_EmptyIsNoop(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	sender := NewHTTPSender(server.URL)
	if err := sender.Send(nil); err != nil {
		t.Errorf("empty Send should be nil error, got: %v", err)
	}
	if called {
		t.Error("empty Send should not hit the network")
	}
}

func TestHTTPSender_Send_5xxIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	sender := NewHTTPSender(server.URL)
	err := sender.Send([]Event{{Command: "send"}})
	if err == nil {
		t.Fatal("expected error from 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention status, got: %v", err)
	}
}

func TestHTTPSender_Send_429IsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(server.Close)

	sender := NewHTTPSender(server.URL)
	if err := sender.Send([]Event{{Command: "x"}}); err == nil {
		t.Fatal("expected error from 429")
	}
}

func TestHTTPSender_TrimsTrailingSlashFromBaseURL(t *testing.T) {
	sender := NewHTTPSender("https://api.example.com/api/")
	if sender.BaseURL != "https://api.example.com/api" {
		t.Errorf("trailing slash not trimmed, got %q", sender.BaseURL)
	}
}
