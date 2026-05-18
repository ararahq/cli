package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newOAuthTestClient(baseURL string) *Client {
	client := newTestClient(baseURL)
	return client
}

func TestRequestDeviceCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/oauth/device/code" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deviceCode":"D1","userCode":"BXTZ-9KQM","verificationUri":"https://ararahq.com/cli/auth?code=BXTZ-9KQM","expiresIn":600,"interval":5}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	got, err := client.RequestDeviceCode("", "")
	if err != nil {
		t.Fatalf("RequestDeviceCode: %v", err)
	}
	if got.UserCode != "BXTZ-9KQM" || got.DeviceCode != "D1" {
		t.Errorf("unexpected response: %+v", got)
	}
	if got.ExpiresIn != 600 || got.Interval != 5 {
		t.Errorf("expected expires=600 interval=5, got %+v", got)
	}
}

func TestRequestDeviceCode_404WithoutErrorCodeMeansEndpointMissing(t *testing.T) {
	// Spring's default 404 page (when no controller maps the route) returns
	// HTML or an unstructured body. We treat that as "OAuth not deployed yet".
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<html><body>404 Not Found</body></html>`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.RequestDeviceCode("", "")
	if !errors.Is(err, ErrOAuthNotSupported) {
		t.Errorf("expected ErrOAuthNotSupported when endpoint missing, got: %v", err)
	}
}

func TestRequestDeviceCode_404WithStructuredCodeIsDomainError(t *testing.T) {
	// 404 WITH a {error:{code,message}} envelope is a legitimate domain
	// error (e.g. device_code_not_found). It must NOT collapse into
	// ErrOAuthNotSupported — that would mask a real bug for the user.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"device_code_not_found","message":"unknown device code"}}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.RequestDeviceCode("", "")
	if errors.Is(err, ErrOAuthNotSupported) {
		t.Errorf("structured 404 should NOT collapse to ErrOAuthNotSupported, got: %v", err)
	}
	if err == nil || !contains(err.Error(), "device_code_not_found") {
		t.Errorf("expected domain error containing the API code, got: %v", err)
	}
}

func TestPollDeviceToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at_xxx","refreshToken":"rt_xxx","tokenType":"Bearer","expiresIn":3600}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	options := PollOptions{
		DeviceCode: "D1",
		Interval:   time.Millisecond,
		ExpiresIn:  time.Second,
		Sleep:      func(time.Duration) {},
	}

	got, err := client.PollDeviceToken(options)
	if err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}
	if got.AccessToken != "at_xxx" {
		t.Errorf("expected accessToken at_xxx, got %q", got.AccessToken)
	}
	if got.RefreshToken != "rt_xxx" {
		t.Errorf("expected refreshToken rt_xxx, got %q", got.RefreshToken)
	}
}

func TestPollDeviceToken_PendingThenSuccess(t *testing.T) {
	calls := atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := calls.Add(1)
		if count < 3 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"authorization_pending","message":"user has not approved"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at_ok","tokenType":"Bearer"}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	got, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Millisecond,
		ExpiresIn:  time.Second,
		Sleep:      func(time.Duration) {},
	})
	if err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}
	if got.AccessToken != "at_ok" {
		t.Errorf("expected at_ok, got %q", got.AccessToken)
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 polls, got %d", calls.Load())
	}
}

func TestPollDeviceToken_SlowDownIncreasesInterval(t *testing.T) {
	calls := atomic.Int32{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		count := calls.Add(1)
		if count == 1 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"slow_down","message":"polling too fast"}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at_x"}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	slowDownCalls := atomic.Int32{}

	if _, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Second,
		ExpiresIn:  10 * time.Second,
		Sleep:      func(time.Duration) {},
		OnSlowDown: func(time.Duration) { slowDownCalls.Add(1) },
	}); err != nil {
		t.Fatalf("PollDeviceToken: %v", err)
	}
	if slowDownCalls.Load() != 1 {
		t.Errorf("expected OnSlowDown to fire once, got %d", slowDownCalls.Load())
	}
}

func TestPollDeviceToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"access_denied","message":"user denied"}}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Millisecond,
		ExpiresIn:  time.Second,
		Sleep:      func(time.Duration) {},
	})
	if err == nil || !contains(err.Error(), "denied") {
		t.Errorf("expected denial error, got: %v", err)
	}
}

func TestPollDeviceToken_ExpiredCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"expired_token","message":"too late"}}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Millisecond,
		ExpiresIn:  time.Second,
		Sleep:      func(time.Duration) {},
	})
	if err == nil || !contains(err.Error(), "expired") {
		t.Errorf("expected expiry error, got: %v", err)
	}
}

func TestPollDeviceToken_UnknownError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"surprise","message":"unexpected"}}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Millisecond,
		ExpiresIn:  time.Second,
		Sleep:      func(time.Duration) {},
	})
	if err == nil {
		t.Fatal("expected error on unknown oauth code, got nil")
	}
}

func TestPollDeviceToken_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"authorization_pending","message":"keep waiting"}}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	currentTime := time.Now()
	tickCount := 0

	_, err := client.PollDeviceToken(PollOptions{
		DeviceCode: "D1",
		Interval:   time.Second,
		ExpiresIn:  3 * time.Second,
		Sleep:      func(time.Duration) {},
		Now: func() time.Time {
			tickCount++
			return currentTime.Add(time.Duration(tickCount) * 2 * time.Second)
		},
	})
	if err == nil || !contains(err.Error(), "expired") {
		t.Errorf("expected expiry error from clock advancing past deadline, got: %v", err)
	}
}

func TestPollDeviceToken_RejectsEmptyDeviceCode(t *testing.T) {
	client := newOAuthTestClient("http://x")
	if _, err := client.PollDeviceToken(PollOptions{}); err == nil {
		t.Error("expected error on empty device code")
	}
}

func TestRefreshDeviceToken_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth/token/refresh" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accessToken":"at_new","expiresIn":3600}`))
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	got, err := client.RefreshDeviceToken("", "rt_xxx")
	if err != nil {
		t.Fatalf("RefreshDeviceToken: %v", err)
	}
	if got.AccessToken != "at_new" {
		t.Errorf("expected at_new, got %q", got.AccessToken)
	}
}

func TestRefreshDeviceToken_RejectsEmpty(t *testing.T) {
	client := newOAuthTestClient("http://x")
	if _, err := client.RefreshDeviceToken("", ""); err == nil {
		t.Error("expected error on empty refresh token")
	}
}

func TestRefreshDeviceToken_404IsNotSupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := newOAuthTestClient(server.URL)
	_, err := client.RefreshDeviceToken("", "rt_xxx")
	if !errors.Is(err, ErrOAuthNotSupported) {
		t.Errorf("expected ErrOAuthNotSupported, got: %v", err)
	}
}

func TestExtractOAuthErrorCode(t *testing.T) {
	if got := extractOAuthErrorCode(errors.New("boom")); got != "" {
		t.Errorf("non-API error should yield empty code, got %q", got)
	}
	apiErr := &APIError{Code: "  authorization_pending  "}
	if got := extractOAuthErrorCode(apiErr); got != "authorization_pending" {
		t.Errorf("expected trimmed code, got %q", got)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
