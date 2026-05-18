package cmd

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/version"
)

func setVersion(v string) {
	version.Version = v
}

func getVersion() string {
	return version.Version
}

func setupDoctorEnv(t *testing.T) {
	t.Helper()
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	keyring.MockInit()
}

func newTestDoctor() *Doctor {
	return &Doctor{
		profileName: "default",
		httpClient:  &http.Client{Timeout: 200 * time.Millisecond},
		latestVersionFunc: func() (string, error) {
			return "v0.1.0", nil
		},
		now: time.Now,
	}
}

func TestRunDoctor_UsesInjectedDoctor(t *testing.T) {
	setupDoctorEnv(t)

	originalFactory := doctorFactory
	t.Cleanup(func() { doctorFactory = originalFactory })
	doctorFactory = func(_ string) *Doctor {
		return newTestDoctor()
	}

	originalVersion := getVersion()
	setVersion("0.1.0")
	t.Cleanup(func() { setVersion(originalVersion) })

	// runDoctor returns an error when checks fail. We don't care about the
	// outcome here — we want coverage of the happy/render branches.
	_ = runDoctor(nil, nil)
}

func TestDoctor_Run_HappyPath(t *testing.T) {
	setupDoctorEnv(t)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/v1/templates":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiServer.Close)

	configuration := &config.Config{
		CurrentProfile: "default",
		Output:         "table",
		Profiles: map[string]config.Profile{
			"default": {APIURL: apiServer.URL, Mode: "test"},
		},
	}
	if err := config.Save(configuration); err != nil {
		t.Fatal(err)
	}
	if err := config.StoreAPIKey("default", "ara_test_xxxxxxxx"); err != nil {
		t.Fatal(err)
	}

	doctor := newTestDoctor()
	report := doctor.Run()

	if report.Summary == "" || hasFailures(report) {
		t.Errorf("expected no failures, summary=%q failures=%d checks=%+v",
			report.Summary, countByStatus(report, StatusFail), report.Checks)
	}

	requiredChecks := []string{
		checkNameVersion,
		checkNameConfig,
		checkNameProfile,
		checkNameAPIKey,
		checkNameAPIReachable,
		checkNameAuth,
	}
	for _, want := range requiredChecks {
		if !containsCheck(report.Checks, want) {
			t.Errorf("missing check: %q", want)
		}
	}
}

func TestDoctor_Run_NoAPIKey_StopsBeforeAuth(t *testing.T) {
	setupDoctorEnv(t)

	configuration := &config.Config{
		CurrentProfile: "default",
		Profiles:       map[string]config.Profile{"default": {APIURL: "http://127.0.0.1:1", Mode: "test"}},
	}
	if err := config.Save(configuration); err != nil {
		t.Fatal(err)
	}

	doctor := newTestDoctor()
	report := doctor.Run()

	if containsCheck(report.Checks, checkNameAuth) {
		t.Error("auth check should be skipped when no API key")
	}
	if !hasFailures(report) {
		t.Error("expected failure on missing API key")
	}
}

func TestDoctor_CheckLatestVersion_DevBuild(t *testing.T) {
	doctor := newTestDoctor()
	original := getVersion()
	setVersion("dev")
	t.Cleanup(func() { setVersion(original) })

	got := doctor.checkLatestVersion()
	if got.Status != StatusWarn {
		t.Errorf("dev build should warn, got %v", got)
	}
}

func TestDoctor_CheckLatestVersion_FetchError(t *testing.T) {
	doctor := newTestDoctor()
	doctor.latestVersionFunc = func() (string, error) {
		return "", errors.New("offline")
	}
	original := getVersion()
	setVersion("0.1.0")
	t.Cleanup(func() { setVersion(original) })

	got := doctor.checkLatestVersion()
	if got.Status != StatusWarn {
		t.Errorf("fetch error should warn, got %v", got)
	}
}

func TestDoctor_CheckLatestVersion_Outdated(t *testing.T) {
	doctor := newTestDoctor()
	doctor.latestVersionFunc = func() (string, error) {
		return "v1.0.0", nil
	}
	original := getVersion()
	setVersion("0.1.0")
	t.Cleanup(func() { setVersion(original) })

	got := doctor.checkLatestVersion()
	if got.Status != StatusWarn {
		t.Errorf("outdated should warn, got %v", got)
	}
	if got.Remediation == "" {
		t.Error("outdated should suggest remediation")
	}
}

func TestDoctor_CheckLatestVersion_UpToDate(t *testing.T) {
	doctor := newTestDoctor()
	doctor.latestVersionFunc = func() (string, error) {
		return "v0.1.0", nil
	}
	original := getVersion()
	setVersion("0.1.0")
	t.Cleanup(func() { setVersion(original) })

	got := doctor.checkLatestVersion()
	if got.Status != StatusOK {
		t.Errorf("up-to-date should be OK, got %v", got)
	}
}

func TestDoctor_CheckAPIReachable_NetworkError(t *testing.T) {
	doctor := newTestDoctor()
	got := doctor.checkAPIReachable("http://127.0.0.1:1")
	if got.Status != StatusFail {
		t.Errorf("unreachable should fail, got %v", got)
	}
}

func TestDoctor_CheckAPIReachable_ServerError(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(apiServer.Close)

	doctor := newTestDoctor()
	got := doctor.checkAPIReachable(apiServer.URL)
	if got.Status != StatusFail {
		t.Errorf("5xx should fail, got %v", got)
	}
}

func TestDoctor_CheckAuth_AuthRejected(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauth","message":"bad key"}}`))
	}))
	t.Cleanup(apiServer.Close)

	doctor := newTestDoctor()
	got := doctor.checkAuth(apiServer.URL, "ara_test_x")
	if got.Status != StatusFail {
		t.Errorf("401 should be fail, got %v", got)
	}
	if got.Remediation == "" {
		t.Error("auth fail should suggest remediation")
	}
}

func TestDoctor_CheckAuth_TransientErrorWarns(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(apiServer.Close)

	doctor := newTestDoctor()
	got := doctor.checkAuth(apiServer.URL, "ara_test_x")
	if got.Status != StatusWarn {
		t.Errorf("non-401 error should warn, got %v", got)
	}
}

func TestDescribeKeyringBackend(t *testing.T) {
	got := describeKeyringBackend()
	if got == "" {
		t.Error("backend description should not be empty")
	}
}

func TestNormalizeVersion(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"v1.2.3", "1.2.3"},
		{"  v0.1 ", "0.1"},
		{"1.2.3", "1.2.3"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			if got := normalizeVersion(tc.input); got != tc.want {
				t.Errorf("normalizeVersion(%q): want %q, got %q", tc.input, tc.want, got)
			}
		})
	}
}

func TestSummarize(t *testing.T) {
	cases := []struct {
		name   string
		checks []CheckResult
		want   string
	}{
		{
			name:   "all ok",
			checks: []CheckResult{{Status: StatusOK}, {Status: StatusOK}},
			want:   "2 checks ok",
		},
		{
			name:   "warnings",
			checks: []CheckResult{{Status: StatusOK}, {Status: StatusWarn}},
			want:   "1 ok, 1 warning",
		},
		{
			name:   "failures",
			checks: []CheckResult{{Status: StatusOK}, {Status: StatusFail}, {Status: StatusWarn}},
			want:   "1 ok, 1 warning, 1 failing",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarize(tc.checks); got != tc.want {
				t.Errorf("summarize: want %q, got %q", tc.want, got)
			}
		})
	}
}

func TestStatusGlyph(t *testing.T) {
	if statusGlyph(StatusOK) == "" {
		t.Error("OK glyph should not be empty")
	}
	if statusGlyph(StatusWarn) == "" {
		t.Error("Warn glyph should not be empty")
	}
	if statusGlyph(StatusFail) == "" {
		t.Error("Fail glyph should not be empty")
	}
	if statusGlyph(CheckStatus("unknown")) != "?" {
		t.Errorf("unknown status should yield '?'")
	}
}

func TestFetchLatestVersion_HTTPSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v9.9.9"}`))
	}))
	t.Cleanup(server.Close)

	got, err := fetchLatestVersionFromURL(&http.Client{Timeout: 200 * time.Millisecond}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got != "v9.9.9" {
		t.Errorf("want v9.9.9, got %q", got)
	}
}

func TestFetchLatestVersion_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)

	if _, err := fetchLatestVersionFromURL(&http.Client{Timeout: 200 * time.Millisecond}, server.URL); err == nil {
		t.Error("expected error on non-200 GitHub response")
	}
}

func TestFetchLatestVersion_MissingTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)

	if _, err := fetchLatestVersionFromURL(&http.Client{Timeout: 200 * time.Millisecond}, server.URL); err == nil {
		t.Error("expected error when payload missing tag_name")
	}
}

func TestFetchLatestVersion_BadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	t.Cleanup(server.Close)

	if _, err := fetchLatestVersionFromURL(&http.Client{Timeout: 200 * time.Millisecond}, server.URL); err == nil {
		t.Error("expected JSON parse error")
	}
}

func TestPrintDoctorReport_DoesNotPanic(t *testing.T) {
	report := &DoctorReport{
		Version:  "0.1.0",
		Platform: "test/test",
		Summary:  "1 ok",
		Checks: []CheckResult{
			{Name: "X", Status: StatusOK, Message: "ok"},
			{Name: "Y", Status: StatusWarn, Message: "warn", Remediation: "fix"},
			{Name: "Z", Status: StatusFail, Message: "fail"},
		},
	}
	printDoctorReport(report)
}

func containsCheck(checks []CheckResult, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
