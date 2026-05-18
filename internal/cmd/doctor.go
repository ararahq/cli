package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/version"
)

const (
	doctorHTTPTimeout     = 5 * time.Second
	doctorReleaseURL      = "https://api.github.com/repos/ararahq/cli/releases/latest"
	checkNameVersion      = "CLI version"
	checkNameConfig       = "Config file"
	checkNameProfile      = "Active profile"
	checkNameAPIKey       = "API key present"
	checkNameAPIReachable = "API reachable"
	checkNameAuth         = "Authentication"
	keyringMacOS          = "macOS Keychain"
	keyringLinux          = "Secret Service (libsecret)"
	keyringWindows        = "Windows Credential Manager"
	keyringFallback       = "config-file fallback"
)

type CheckStatus string

const (
	StatusOK   CheckStatus = "ok"
	StatusWarn CheckStatus = "warn"
	StatusFail CheckStatus = "fail"
)

type CheckResult struct {
	Name        string        `json:"name"`
	Status      CheckStatus   `json:"status"`
	Message     string        `json:"message"`
	Remediation string        `json:"remediation,omitempty"`
	LatencyMS   int64         `json:"latencyMs,omitempty"`
	latency     time.Duration `json:"-"`
}

type DoctorReport struct {
	Version     string        `json:"version"`
	Platform    string        `json:"platform"`
	GeneratedAt time.Time     `json:"generatedAt"`
	Checks      []CheckResult `json:"checks"`
	Summary     string        `json:"summary"`
}

type Doctor struct {
	profileName       string
	httpClient        *http.Client
	latestVersionFunc func() (string, error)
	now               func() time.Time
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose the CLI environment and connection",
	Long: "Run a checklist against your local CLI: config, credentials, keyring backend,\n" +
		"connectivity to the AraraHQ API, authentication validity, and the latest\n" +
		"available release. Output is human-readable by default; use -o json for\n" +
		"machine output (pipe-friendly for support bundles).",
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

// doctorFactory builds the Doctor used by runDoctor. Indirection so tests
// can stub the latest-version fetcher and HTTP client without spinning up
// the real GitHub release flow.
var doctorFactory = newDefaultDoctor

func runDoctor(_ *cobra.Command, _ []string) error {
	doctor := doctorFactory(GetProfile())
	report := doctor.Run()

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(report)
	}

	printDoctorReport(report)

	if hasFailures(report) {
		return fmt.Errorf("doctor reported %d failing checks", countByStatus(report, StatusFail))
	}
	return nil
}

func newDefaultDoctor(profileName string) *Doctor {
	httpClient := &http.Client{Timeout: doctorHTTPTimeout}
	return &Doctor{
		profileName:       profileName,
		httpClient:        httpClient,
		latestVersionFunc: func() (string, error) { return fetchLatestVersionFromURL(httpClient, doctorReleaseURL) },
		now:               time.Now,
	}
}

func (doctor *Doctor) Run() *DoctorReport {
	report := &DoctorReport{
		Version:     version.Version,
		Platform:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		GeneratedAt: doctor.now(),
	}

	versionCheck := doctor.checkLatestVersion()
	report.Checks = append(report.Checks, versionCheck)

	configResult, configuration := doctor.checkConfigFile()
	report.Checks = append(report.Checks, configResult)
	if configuration == nil {
		report.Summary = summarize(report.Checks)
		return report
	}

	profileResult, activeProfile := doctor.checkActiveProfile(configuration)
	report.Checks = append(report.Checks, profileResult)
	if activeProfile == nil {
		report.Summary = summarize(report.Checks)
		return report
	}

	apiKeyResult, apiKey := doctor.checkAPIKey()
	report.Checks = append(report.Checks, apiKeyResult)

	report.Checks = append(report.Checks, doctor.checkAPIReachable(activeProfile.APIURL))

	if apiKey != "" {
		report.Checks = append(report.Checks, doctor.checkAuth(activeProfile.APIURL, apiKey))
	}

	report.Summary = summarize(report.Checks)
	return report
}

func (doctor *Doctor) checkLatestVersion() CheckResult {
	current := version.Version
	if current == "" || current == "dev" {
		return CheckResult{
			Name:    checkNameVersion,
			Status:  StatusWarn,
			Message: "running a dev build (no version metadata)",
		}
	}

	latest, fetchErr := doctor.latestVersionFunc()
	if fetchErr != nil {
		return CheckResult{
			Name:    checkNameVersion,
			Status:  StatusWarn,
			Message: fmt.Sprintf("running v%s — could not check latest (%s)", current, fetchErr.Error()),
		}
	}

	if normalizeVersion(current) == normalizeVersion(latest) {
		return CheckResult{
			Name:    checkNameVersion,
			Status:  StatusOK,
			Message: fmt.Sprintf("v%s (latest)", current),
		}
	}

	return CheckResult{
		Name:        checkNameVersion,
		Status:      StatusWarn,
		Message:     fmt.Sprintf("v%s installed, v%s available", current, latest),
		Remediation: "run 'arara upgrade' to update",
	}
}

func (doctor *Doctor) checkConfigFile() (CheckResult, *config.Config) {
	configuration, loadErr := config.Load()
	if loadErr != nil {
		return CheckResult{
			Name:        checkNameConfig,
			Status:      StatusFail,
			Message:     loadErr.Error(),
			Remediation: fmt.Sprintf("inspect %s and fix or delete", config.ConfigPath()),
		}, nil
	}
	return CheckResult{
		Name:    checkNameConfig,
		Status:  StatusOK,
		Message: config.ConfigPath(),
	}, configuration
}

func (doctor *Doctor) checkActiveProfile(configuration *config.Config) (CheckResult, *config.Profile) {
	activeProfile, profileErr := config.GetActiveProfile(configuration)
	if profileErr != nil {
		return CheckResult{
			Name:        checkNameProfile,
			Status:      StatusFail,
			Message:     profileErr.Error(),
			Remediation: "run 'arara login' or 'arara config set current_profile <name>'",
		}, nil
	}
	return CheckResult{
		Name:    checkNameProfile,
		Status:  StatusOK,
		Message: fmt.Sprintf("%s (mode=%s, api=%s)", configuration.CurrentProfile, activeProfile.Mode, activeProfile.APIURL),
	}, activeProfile
}

func (doctor *Doctor) checkAPIKey() (CheckResult, string) {
	apiKey, keyErr := config.GetAPIKey(doctor.profileName)
	if keyErr != nil || apiKey == "" {
		return CheckResult{
			Name:        checkNameAPIKey,
			Status:      StatusFail,
			Message:     fmt.Sprintf("no key found for profile %q", doctor.profileName),
			Remediation: "run 'arara login'",
		}, ""
	}

	keyringLabel := describeKeyringBackend()
	return CheckResult{
		Name:    checkNameAPIKey,
		Status:  StatusOK,
		Message: fmt.Sprintf("present (storage: %s)", keyringLabel),
	}, apiKey
}

func (doctor *Doctor) checkAPIReachable(apiURL string) CheckResult {
	// HealthController is mounted at /health (no /v1 prefix) and is in the
	// SecurityConfig permitAll list. With the backend's context-path of
	// /api, the absolute URL ends up as https://api.ararahq.com/api/health.
	healthURL := strings.TrimRight(apiURL, "/") + "/health"

	start := doctor.now()
	httpResponse, requestErr := doctor.httpClient.Get(healthURL)
	latency := doctor.now().Sub(start)

	if requestErr != nil {
		return CheckResult{
			Name:        checkNameAPIReachable,
			Status:      StatusFail,
			Message:     requestErr.Error(),
			Remediation: "check your network or status.ararahq.com",
			latency:     latency,
			LatencyMS:   latency.Milliseconds(),
		}
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode >= 500 {
		return CheckResult{
			Name:      checkNameAPIReachable,
			Status:    StatusFail,
			Message:   fmt.Sprintf("upstream returned %d (%s)", httpResponse.StatusCode, httpResponse.Status),
			latency:   latency,
			LatencyMS: latency.Milliseconds(),
		}
	}

	return CheckResult{
		Name:      checkNameAPIReachable,
		Status:    StatusOK,
		Message:   fmt.Sprintf("%s (%dms)", apiURL, latency.Milliseconds()),
		latency:   latency,
		LatencyMS: latency.Milliseconds(),
	}
}

func (doctor *Doctor) checkAuth(apiURL, apiKey string) CheckResult {
	client := api.NewClient(apiURL, apiKey)
	if _, err := client.ListTemplates(); err != nil {
		if api.IsAuthError(err) {
			return CheckResult{
				Name:        checkNameAuth,
				Status:      StatusFail,
				Message:     "API key was rejected",
				Remediation: "run 'arara login' with a fresh key from ararahq.com/dashboard",
			}
		}
		return CheckResult{
			Name:    checkNameAuth,
			Status:  StatusWarn,
			Message: fmt.Sprintf("could not validate (%s)", err.Error()),
		}
	}

	return CheckResult{
		Name:    checkNameAuth,
		Status:  StatusOK,
		Message: "key accepted by API",
	}
}

func describeKeyringBackend() string {
	switch runtime.GOOS {
	case "darwin":
		return keyringMacOS
	case "linux":
		return keyringLinux
	case "windows":
		return keyringWindows
	default:
		return keyringFallback
	}
}

func fetchLatestVersionFromURL(httpClient *http.Client, releaseURL string) (string, error) {
	httpRequest, requestErr := http.NewRequest(http.MethodGet, releaseURL, nil)
	if requestErr != nil {
		return "", requestErr
	}
	httpRequest.Header.Set("Accept", "application/vnd.github+json")
	httpRequest.Header.Set("User-Agent", "arara-cli-doctor")

	httpResponse, responseErr := httpClient.Do(httpRequest)
	if responseErr != nil {
		return "", responseErr
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github returned %d", httpResponse.StatusCode)
	}

	var payload struct {
		TagName string `json:"tag_name"`
	}
	if decodeErr := json.NewDecoder(httpResponse.Body).Decode(&payload); decodeErr != nil {
		return "", decodeErr
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("release payload missing tag_name")
	}
	return payload.TagName, nil
}

func normalizeVersion(raw string) string {
	return strings.TrimPrefix(strings.TrimSpace(raw), "v")
}

func summarize(checks []CheckResult) string {
	okCount := countByStatus(&DoctorReport{Checks: checks}, StatusOK)
	warnCount := countByStatus(&DoctorReport{Checks: checks}, StatusWarn)
	failCount := countByStatus(&DoctorReport{Checks: checks}, StatusFail)

	switch {
	case failCount > 0:
		return fmt.Sprintf("%d ok, %d warning, %d failing", okCount, warnCount, failCount)
	case warnCount > 0:
		return fmt.Sprintf("%d ok, %d warning", okCount, warnCount)
	default:
		return fmt.Sprintf("%d checks ok", okCount)
	}
}

func countByStatus(report *DoctorReport, status CheckStatus) int {
	count := 0
	for _, check := range report.Checks {
		if check.Status == status {
			count++
		}
	}
	return count
}

func hasFailures(report *DoctorReport) bool {
	return countByStatus(report, StatusFail) > 0
}

func printDoctorReport(report *DoctorReport) {
	fmt.Fprintf(os.Stdout, "arara doctor — v%s on %s\n\n", report.Version, report.Platform)

	for _, check := range report.Checks {
		fmt.Fprintf(os.Stdout, "%s %-22s %s\n", statusGlyph(check.Status), check.Name, check.Message)
		if check.Remediation != "" {
			fmt.Fprintf(os.Stdout, "  %s %s\n", output.DimStyle.Render("→"), check.Remediation)
		}
	}

	fmt.Fprintf(os.Stdout, "\n%s\n", report.Summary)
}

func statusGlyph(status CheckStatus) string {
	switch status {
	case StatusOK:
		return output.SuccessStyle.Render("✔")
	case StatusWarn:
		return output.WarningStyle.Render("!")
	case StatusFail:
		return output.ErrorStyle.Render("✘")
	default:
		return "?"
	}
}
