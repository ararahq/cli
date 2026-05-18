package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateWebhookListener_GoldenPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != webhookListenersPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id":"lst_8f3e2d",
			"secret":"wh_cli_4f8a2b9e3c1d",
			"organizationId":"org_acme",
			"expiresAt":"2026-05-11T19:30:00Z",
			"heartbeatIntervalSeconds":30
		}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	resp, err := client.CreateWebhookListener(CreateWebhookListenerRequest{Owner: "darwin/arm64@host"})
	if err != nil {
		t.Fatalf("CreateWebhookListener: %v", err)
	}
	if resp.ID != "lst_8f3e2d" || resp.Secret != "wh_cli_4f8a2b9e3c1d" {
		t.Errorf("response mismatch: %+v", resp)
	}
	if resp.HeartbeatIntervalSeconds != 30 {
		t.Errorf("heartbeat interval: %d", resp.HeartbeatIntervalSeconds)
	}
}

func TestCreateWebhookListener_404FallsBackGracefully(t *testing.T) {
	// Bare 404 (no JSON body) means the controller isn't deployed yet —
	// CLI should get ErrCaptureNotSupported and fall back to passive mode.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<html>404 Not Found</html>`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	_, err := client.CreateWebhookListener(CreateWebhookListenerRequest{})
	if !errors.Is(err, ErrCaptureNotSupported) {
		t.Errorf("expected ErrCaptureNotSupported, got: %v", err)
	}
}

func TestCreateWebhookListener_409ReturnsTypedConflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{
			"error":{
				"code":"capture_listener_active",
				"message":"another CLI session is already capturing for this org",
				"details":{
					"listenerId":"lst_existing",
					"owner":"darwin/arm64@other-host",
					"startedAt":"2026-05-11T18:30:00Z",
					"expiresAt":"2026-05-11T19:30:00Z"
				}
			}
		}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	_, err := client.CreateWebhookListener(CreateWebhookListenerRequest{})

	var conflict *CaptureConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected *CaptureConflictError, got: %T (%v)", err, err)
	}
	if conflict.ListenerID != "lst_existing" {
		t.Errorf("listenerId: %q", conflict.ListenerID)
	}
	if conflict.Owner != "darwin/arm64@other-host" {
		t.Errorf("owner: %q", conflict.Owner)
	}
	if conflict.ExpiresAt != "2026-05-11T19:30:00Z" {
		t.Errorf("expiresAt: %q", conflict.ExpiresAt)
	}
	if !strings.Contains(conflict.Error(), "lst_existing") {
		t.Errorf("Error() should reference listener id, got: %v", conflict)
	}
}

func TestCreateWebhookListener_404WithStructuredCodeIsDomainError(t *testing.T) {
	// 404 WITH error.code is a real domain error (e.g. org not found),
	// not "feature missing". Must NOT collapse to ErrCaptureNotSupported.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"organization_not_found","message":"unknown org"}}`))
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	_, err := client.CreateWebhookListener(CreateWebhookListenerRequest{})
	if errors.Is(err, ErrCaptureNotSupported) {
		t.Errorf("structured 404 should NOT collapse to ErrCaptureNotSupported, got: %v", err)
	}
}

func TestHeartbeatWebhookListener_204IsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/heartbeat") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	if err := client.HeartbeatWebhookListener("lst_x"); err != nil {
		t.Errorf("heartbeat: %v", err)
	}
}

func TestReleaseWebhookListener_204IsSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := newTestClient(server.URL)
	if err := client.ReleaseWebhookListener("lst_x"); err != nil {
		t.Errorf("release: %v", err)
	}
}

func TestStringFromDetails_HandlesMissingAndPresent(t *testing.T) {
	apiErr := &APIError{
		RawBody: `{"error":{"code":"x","details":{"foo":"bar","baz":"qux"}}}`,
	}
	if got := stringFromDetails(apiErr, "foo"); got != "bar" {
		t.Errorf("foo: want bar, got %q", got)
	}
	if got := stringFromDetails(apiErr, "baz"); got != "qux" {
		t.Errorf("baz: want qux, got %q", got)
	}
	if got := stringFromDetails(apiErr, "missing"); got != "" {
		t.Errorf("missing: want empty, got %q", got)
	}
}

func TestStringFromDetails_EmptyBodyReturnsEmpty(t *testing.T) {
	apiErr := &APIError{}
	if got := stringFromDetails(apiErr, "foo"); got != "" {
		t.Errorf("empty body should return empty, got %q", got)
	}
}

func TestIndexOf(t *testing.T) {
	if indexOf("hello world", "world") != 6 {
		t.Errorf("simple match")
	}
	if indexOf("hello", "world") != -1 {
		t.Errorf("no match should return -1")
	}
	if indexOf("", "x") != -1 {
		t.Errorf("empty haystack")
	}
	if indexOf("xx", "") != 0 {
		t.Errorf("empty needle should return 0")
	}
}
