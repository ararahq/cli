package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ararahq/cli/internal/api"
)

func TestRunSendWatch_ReturnsOnTerminalSuccess(t *testing.T) {
	statuses := []string{"queued", "sent", "delivered"}
	var pollIndex atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/messages/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		index := int(pollIndex.Add(1)) - 1
		if index >= len(statuses) {
			index = len(statuses) - 1
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","status":"` + statuses[index] + `"}`))
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendWatchTimeoutFlag = 5 * time.Second

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	initial := &api.MessageResponse{ID: "msg_1", Status: "queued", Receiver: "+5511"}

	if err := runSendWatch(client, initial); err != nil {
		t.Fatalf("runSendWatch returned error: %v", err)
	}
	if pollIndex.Load() < 2 {
		t.Errorf("expected at least 2 polls, got %d", pollIndex.Load())
	}
}

func TestRunSendWatch_ReturnsErrorOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","status":"failed"}`))
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendWatchTimeoutFlag = 5 * time.Second

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	initial := &api.MessageResponse{ID: "msg_1", Status: "queued"}

	err := runSendWatch(client, initial)
	if err == nil {
		t.Fatal("expected error on terminal failure status, got nil")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Errorf("error should mention failure status, got: %v", err)
	}
}

func TestRunSendWatch_ReturnsImmediatelyIfAlreadyTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendWatchTimeoutFlag = 5 * time.Second

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	initial := &api.MessageResponse{ID: "msg_1", Status: "delivered"}

	start := time.Now()
	if err := runSendWatch(client, initial); err != nil {
		t.Fatalf("runSendWatch should succeed without polling, got: %v", err)
	}
	if time.Since(start) > watchInitialPollInterval {
		t.Errorf("should have returned without polling, took %v", time.Since(start))
	}
}

func TestRunSendWatch_TimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","status":"queued"}`))
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendWatchTimeoutFlag = 100 * time.Millisecond

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	initial := &api.MessageResponse{ID: "msg_1", Status: "queued"}

	err := runSendWatch(client, initial)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout in error message, got: %v", err)
	}
}

func TestRunSendDryRun_TemplateWithCostEstimate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/campaigns/estimate") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"templateCost":1.0,"unitPrice":1.5,"totalCost":1.5,"templateCategory":"MARKETING","recipientCount":1}`))
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendDryRunFlag = true

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	request := api.SendMessageRequest{
		Receiver:     "whatsapp:+5511",
		TemplateName: "hello",
	}

	if err := runSendDryRun(client, request); err != nil {
		t.Fatalf("runSendDryRun: %v", err)
	}
}

func TestRunSendDryRun_FreeformSkipsEstimate(t *testing.T) {
	estimateCalled := atomic.Bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/campaigns/estimate") {
			estimateCalled.Store(true)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	restoreSendFlags(t)
	sendDryRunFlag = true

	client := api.NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	request := api.SendMessageRequest{
		Receiver: "whatsapp:+5511",
		Body:     "hello",
	}

	if err := runSendDryRun(client, request); err != nil {
		t.Fatalf("runSendDryRun: %v", err)
	}
	if estimateCalled.Load() {
		t.Error("freeform message should not call estimate endpoint")
	}
}
