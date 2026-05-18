package cmd

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ararahq/cli/internal/api"
)

// withFakeAPIClient swaps newClientForCmd for the duration of a test so the
// Cobra wrappers (runFooList, runFooCreate, ...) construct a httptest-backed
// client instead of trying to read the real keyring/config. Restores the
// original on cleanup.
func withFakeAPIClient(t *testing.T, client *api.Client) {
	t.Helper()
	original := newClientForCmd
	newClientForCmd = func() (*api.Client, error) { return client, nil }
	t.Cleanup(func() { newClientForCmd = original })
}

// fakeAPIServer spins up an httptest.Server using the supplied handler and
// returns a *api.Client wired to it. Use t.Cleanup-style closing implicit
// via httptest.NewServer's Close on test completion.
func fakeAPIServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *api.Client) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := api.NewClient(server.URL, "ara_test_unit-test-key-123456")
	return server, client
}

// fakeAPIServerJSON is the most common shape: respond to any request with
// `status` and the given JSON body. Use this for golden-path tests.
func fakeAPIServerJSON(t *testing.T, status int, jsonBody string) (*httptest.Server, *api.Client) {
	return fakeAPIServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(jsonBody))
	})
}
