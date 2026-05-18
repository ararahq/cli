package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/output"
)

const (
	apiKeyMinLength       = 12
	maskedKeyVisibleChars = 4
	// #nosec G101 -- UX prompt string, not a credential.
	apiKeyPrompt   = "Enter your API key: "
	oauthEnableEnv = "ARARA_OAUTH_ENABLED"
)

var (
	loginAPIKeyFlag       string
	loginOAuthFlag        bool
	loginOAuthClientIDEnv = "ARARA_OAUTH_CLIENT_ID"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the AraraHQ API",
	Long: "Authenticate via OAuth 2.0 Device Authorization (browser-based, default).\n" +
		"Pass --key <key> to authenticate with an API key directly — recommended\n" +
		"only for CI/automation. Credentials are stored in your OS keyring with\n" +
		"config file fallback.",
	RunE: runLogin,
}

func init() {
	loginCmd.Flags().StringVar(&loginAPIKeyFlag, "key", "", "API key for non-interactive auth (CI/automation). Default is browser OAuth.")
	loginCmd.Flags().BoolVar(&loginOAuthFlag, "oauth", false, "deprecated: OAuth is now the default. Flag kept for backward compat.")
	loginCmd.Flags().BoolVar(&loginOAuthFlag, "experimental-oauth", false, "deprecated alias for --oauth")
	_ = loginCmd.Flags().MarkHidden("oauth")
	_ = loginCmd.Flags().MarkHidden("experimental-oauth")
	rootCmd.AddCommand(loginCmd)
}

// oauthFlagEnabled is retained for backward compatibility with tests; the gate
// has been removed now that the backend implements /oauth/device/*.
func oauthFlagEnabled() bool {
	return os.Getenv(oauthEnableEnv) != "0"
}

func runLogin(command *cobra.Command, arguments []string) error {
	profileName := GetProfile()

	// OAuth Device Flow is the default. API key path requires explicit --key
	// or piping a key via stdin (CI scripts). This matches the modern pattern
	// used by gh, stripe, npm — keep API key as escape hatch, not default.
	if loginAPIKeyFlag == "" && !isStdinPiped() {
		return runOAuthDeviceFlow(profileName)
	}

	apiKey, readError := resolveLoginAPIKey()
	if readError != nil {
		return readError
	}

	if validationError := validateAPIKeyFormat(apiKey); validationError != nil {
		output.PrintError(validationError.Error())
		return validationError
	}

	if verifyError := verifyAPIKeyWithServer(apiKey); verifyError != nil {
		output.PrintError(fmt.Sprintf("API key validation failed: %s", verifyError.Error()))
		return verifyError
	}

	if storeError := persistCredentials(profileName, apiKey); storeError != nil {
		output.PrintError(fmt.Sprintf("Failed to save credentials: %s", storeError.Error()))
		return storeError
	}

	maskedKey := maskAPIKey(apiKey)
	output.PrintSuccess(fmt.Sprintf("Authenticated successfully! Profile: %s %s %s", profileName, output.DimStyle.Render("--"), maskedKey))

	return nil
}

func runOAuthDeviceFlow(profileName string) error {
	apiURL := resolveOAuthAPIURL(profileName)
	clientID := strings.TrimSpace(os.Getenv(loginOAuthClientIDEnv))
	apiClient := api.NewClient(apiURL, "")

	deviceCode, deviceError := apiClient.RequestDeviceCode(clientID, "cli:full")
	if deviceError != nil {
		return printOAuthError(deviceError)
	}

	printDeviceCodePrompt(deviceCode)

	tokenResponse, pollError := apiClient.PollDeviceToken(buildPollOptions(clientID, deviceCode))
	if pollError != nil {
		return printOAuthError(pollError)
	}

	if storeError := persistOAuthToken(profileName, apiURL, tokenResponse); storeError != nil {
		output.PrintError(fmt.Sprintf("Failed to save token: %s", storeError.Error()))
		return storeError
	}

	output.PrintSuccess(fmt.Sprintf("Authenticated via OAuth! Profile: %s", profileName))
	if tokenResponse.ExpiresIn > 0 {
		output.PrintInfo(fmt.Sprintf("Access token expires in %d seconds. Re-run 'arara login' when it does.", tokenResponse.ExpiresIn))
	}
	return nil
}

// resolveOAuthAPIURL returns the API URL of the profile if known, otherwise
// the package default. We never block on config load failure here — OAuth
// can still run against the default URL.
func resolveOAuthAPIURL(profileName string) string {
	configuration, loadError := config.Load()
	if loadError != nil {
		return config.DefaultAPIURL
	}
	if profile, exists := configuration.Profiles[profileName]; exists && profile.APIURL != "" {
		return profile.APIURL
	}
	return config.DefaultAPIURL
}

// printDeviceCodePrompt renders the user-facing instructions: a URL to
// visit and a code to enter. Prefers verificationUriComplete when the
// backend supplies it (RFC 8628 §3.3.1) so the form is pre-populated.
func printDeviceCodePrompt(deviceCode *api.DeviceCodeResponse) {
	output.PrintInfo("Open this URL in your browser to approve the request:")
	approveURL := deviceCode.VerificationURIComplete
	if approveURL == "" {
		approveURL = deviceCode.VerificationURI
	}
	fmt.Fprintf(os.Stderr, "  %s\n\n", output.BoldStyle.Render(approveURL))
	output.PrintInfo("Enter this code when prompted:")
	fmt.Fprintf(os.Stderr, "  %s\n\n", output.BoldStyle.Render(deviceCode.UserCode))
	output.PrintInfo(fmt.Sprintf("Waiting for approval (expires in %ds)...", deviceCode.ExpiresIn))
}

func buildPollOptions(clientID string, deviceCode *api.DeviceCodeResponse) api.PollOptions {
	return api.PollOptions{
		ClientID:   clientID,
		DeviceCode: deviceCode.DeviceCode,
		Interval:   time.Duration(deviceCode.Interval) * time.Second,
		ExpiresIn:  time.Duration(deviceCode.ExpiresIn) * time.Second,
		OnSlowDown: func(newInterval time.Duration) {
			output.PrintWarning(fmt.Sprintf("Server asked us to slow down — polling every %v", newInterval))
		},
	}
}

func printOAuthError(err error) error {
	if errors.Is(err, api.ErrOAuthNotSupported) {
		output.PrintError(err.Error())
		return err
	}
	output.PrintError(fmt.Sprintf("OAuth login failed: %s", err.Error()))
	return err
}

func persistOAuthToken(profileName, apiURL string, tokenResponse *api.TokenResponse) error {
	if storeError := config.StoreAPIKey(profileName, tokenResponse.AccessToken); storeError != nil {
		return fmt.Errorf("failed to store access token: %w", storeError)
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		return fmt.Errorf("failed to load configuration: %w", loadError)
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		profile = config.Profile{Mode: config.DefaultMode}
	}
	profile.APIURL = apiURL
	profile.AuthType = config.AuthTypeOAuth
	configuration.Profiles[profileName] = profile
	configuration.CurrentProfile = profileName

	if saveError := config.Save(configuration); saveError != nil {
		return fmt.Errorf("failed to save configuration: %w", saveError)
	}

	return nil
}

func resolveLoginAPIKey() (string, error) {
	if loginAPIKeyFlag != "" {
		return strings.TrimSpace(loginAPIKeyFlag), nil
	}

	return readAPIKeyFromTerminal()
}

// isStdinPiped detects CI/automation scenarios where the user wants to pipe an
// API key into the CLI (e.g. `echo $ARARA_API_KEY | arara login`). In that case
// we skip the OAuth flow and treat the piped input as the key.
func isStdinPiped() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}

func readAPIKeyFromTerminal() (string, error) {
	fmt.Fprint(os.Stderr, apiKeyPrompt)

	fileDescriptor := int(os.Stdin.Fd())

	if term.IsTerminal(fileDescriptor) {
		rawBytes, readError := term.ReadPassword(fileDescriptor)
		fmt.Fprintln(os.Stderr)

		if readError != nil {
			return "", fmt.Errorf("failed to read API key from terminal: %w", readError)
		}

		return strings.TrimSpace(string(rawBytes)), nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("no input received — please provide your API key")
	}

	return strings.TrimSpace(scanner.Text()), nil
}

func validateAPIKeyFormat(apiKey string) error {
	if len(apiKey) < apiKeyMinLength {
		return fmt.Errorf("API key is too short — expected at least %d characters", apiKeyMinLength)
	}

	if !strings.HasPrefix(apiKey, api.KeyPrefixLive) && !strings.HasPrefix(apiKey, api.KeyPrefixTest) {
		return fmt.Errorf("invalid API key format — key must start with %q or %q", api.KeyPrefixLive, api.KeyPrefixTest)
	}

	return nil
}

func verifyAPIKeyWithServer(apiKey string) error {
	client := api.NewClient(config.DefaultAPIURL, apiKey)

	_, templatesError := client.ListTemplates()
	if templatesError == nil {
		return nil
	}

	if api.IsAuthError(templatesError) {
		return fmt.Errorf("invalid or expired API key — please check your key at ararahq.com/dashboard")
	}

	output.PrintWarning("Could not verify key with server (network or server issue). Saving key based on format validation.")
	return nil
}

func persistCredentials(profileName string, apiKey string) error {
	if storeError := config.StoreAPIKey(profileName, apiKey); storeError != nil {
		return fmt.Errorf("failed to store API key in keyring: %w", storeError)
	}

	configuration, loadError := config.Load()
	if loadError != nil {
		return fmt.Errorf("failed to load configuration: %w", loadError)
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		profile = config.Profile{
			APIURL: config.DefaultAPIURL,
			Mode:   config.DefaultMode,
		}
	}

	modeFromKey := detectModeFromKey(apiKey)
	profile.Mode = modeFromKey
	configuration.Profiles[profileName] = profile
	configuration.CurrentProfile = profileName

	if saveError := config.Save(configuration); saveError != nil {
		return fmt.Errorf("failed to save configuration: %w", saveError)
	}

	return nil
}

func detectModeFromKey(apiKey string) string {
	if strings.HasPrefix(apiKey, api.KeyPrefixLive) {
		return "live"
	}

	return "test"
}

func maskAPIKey(apiKey string) string {
	if len(apiKey) <= maskedKeyVisibleChars {
		return "****"
	}

	prefix := extractKeyPrefix(apiKey)
	lastFour := apiKey[len(apiKey)-maskedKeyVisibleChars:]

	return fmt.Sprintf("%s****%s", prefix, lastFour)
}

func extractKeyPrefix(apiKey string) string {
	if strings.HasPrefix(apiKey, api.KeyPrefixLive) {
		return api.KeyPrefixLive
	}

	if strings.HasPrefix(apiKey, api.KeyPrefixTest) {
		return api.KeyPrefixTest
	}

	return ""
}
