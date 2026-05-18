package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ararahq/cli/internal/config"
	"github.com/ararahq/cli/internal/version"
)

const (
	DefaultTimeout = 10 * time.Second
	MaxRetries     = 3
	KeyPrefixLive  = "ara_live_"
	KeyPrefixTest  = "ara_test_"

	// DefaultMCPDryRunURL and MCPDryRunKey produce a Client that can be
	// constructed without credentials, suitable only for inspecting tool
	// registration via `arara mcp --dry-run`. The key is deliberately invalid
	// so it cannot be used against a live API.
	DefaultMCPDryRunURL = "https://invalid.local"
	MCPDryRunKey        = "ara_test_DRY_RUN_INSPECT_ONLY"
)

const (
	defaultBackoffBase   = 500 * time.Millisecond
	defaultBackoffCap    = 8 * time.Second
	maxRetryAfter        = 60 * time.Second
	serverErrorThreshold = 500

	contentTypeJSON      = "application/json"
	authorizationHeader  = "Authorization"
	contentTypeHeader    = "Content-Type"
	userAgentHeader      = "User-Agent"
	idempotencyKeyHeader = "Idempotency-Key"
	retryAfterHeader     = "Retry-After"
	bearerPrefix         = "Bearer "
)

var unsafeMethods = map[string]struct{}{
	http.MethodPost:   {},
	http.MethodPut:    {},
	http.MethodPatch:  {},
	http.MethodDelete: {},
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	verbose    bool

	sleep             func(time.Duration)
	randFloat         func() float64
	newIdempotencyKey func() string
	maxRetries        int
	backoffBase       time.Duration
	backoffCap        time.Duration
}

func NewClient(baseURL string, apiKey string) *Client {
	return &Client{
		baseURL:           strings.TrimRight(baseURL, "/"),
		apiKey:            apiKey,
		httpClient:        &http.Client{Timeout: DefaultTimeout},
		sleep:             time.Sleep,
		randFloat:         mathrand.Float64,
		newIdempotencyKey: uuid.NewString,
		maxRetries:        MaxRetries,
		backoffBase:       defaultBackoffBase,
		backoffCap:        defaultBackoffCap,
	}
}

func NewClientFromConfig() (*Client, error) {
	configuration, loadError := config.Load()
	if loadError != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", loadError)
	}

	activeProfile, profileError := config.GetActiveProfile(configuration)
	if profileError != nil {
		return nil, fmt.Errorf("failed to resolve active profile: %w", profileError)
	}

	apiKey, keyError := resolveAPIKey(configuration.CurrentProfile, activeProfile)
	if keyError != nil {
		return nil, keyError
	}

	return NewClient(activeProfile.APIURL, apiKey), nil
}

func (client *Client) SetVerbose(enabled bool) {
	client.verbose = enabled
}

func (client *Client) IsLiveMode() bool {
	return strings.HasPrefix(client.apiKey, KeyPrefixLive)
}

func (client *Client) Mode() string {
	if client.IsLiveMode() {
		return "LIVE"
	}

	return "TEST"
}

func (client *Client) BaseURL() string {
	return client.baseURL
}

func (client *Client) Do(method string, path string, body any, result any) error {
	return client.DoWithHeaders(method, path, body, result, nil)
}

func (client *Client) Get(path string, result any) error {
	return client.Do(http.MethodGet, path, nil, result)
}

func (client *Client) Post(path string, body any, result any) error {
	return client.Do(http.MethodPost, path, body, result)
}

func (client *Client) Delete(path string) error {
	return client.Do(http.MethodDelete, path, nil, nil)
}

func (client *Client) DoWithHeaders(method string, path string, body any, result any, headers map[string]string) error {
	requestURL := client.baseURL + path

	serializedBody, marshalError := serializeBody(body, client.verbose)
	if marshalError != nil {
		return marshalError
	}

	mergedHeaders := buildRequestHeaders(method, headers, client.newIdempotencyKey)

	var lastError error
	for attempt := range client.maxRetries {
		if client.verbose {
			fmt.Fprintf(os.Stderr, "[DEBUG] %s %s (attempt %d/%d)\n", method, requestURL, attempt+1, client.maxRetries)
		}

		responseBody, executeError := client.executeRequest(method, requestURL, serializedBody, mergedHeaders)
		if executeError == nil {
			return decodeResponse(responseBody, result)
		}

		shouldRetry, retryAfter := classifyForRetry(executeError)
		if !shouldRetry {
			return executeError
		}

		lastError = executeError

		if attempt < client.maxRetries-1 {
			delay := client.computeBackoff(attempt, retryAfter)
			if client.verbose {
				fmt.Fprintf(os.Stderr, "[DEBUG] Retrying in %v due to: %v\n", delay, executeError)
			}
			client.sleep(delay)
		}
	}

	return fmt.Errorf("request failed after %d attempts: %w", client.maxRetries, lastError)
}

func (client *Client) executeRequest(method string, requestURL string, serializedBody []byte, headers map[string]string) ([]byte, error) {
	var requestBodyReader io.Reader
	if serializedBody != nil {
		requestBodyReader = bytes.NewReader(serializedBody)
	}

	httpRequest, requestError := http.NewRequest(method, requestURL, requestBodyReader)
	if requestError != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", requestError)
	}

	httpRequest.Header.Set(authorizationHeader, bearerPrefix+client.apiKey)
	httpRequest.Header.Set(contentTypeHeader, contentTypeJSON)
	httpRequest.Header.Set(userAgentHeader, buildUserAgent())

	for headerName, headerValue := range headers {
		httpRequest.Header.Set(headerName, headerValue)
	}

	httpResponse, responseError := client.httpClient.Do(httpRequest)
	if responseError != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", responseError)
	}
	defer httpResponse.Body.Close()

	responseBytes, readError := io.ReadAll(httpResponse.Body)
	if readError != nil {
		return nil, fmt.Errorf("failed to read response body: %w", readError)
	}

	if client.verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Response status: %d\n", httpResponse.StatusCode)
		fmt.Fprintf(os.Stderr, "[DEBUG] Response body: %s\n", string(RedactJSONForLog(responseBytes)))
	}

	if httpResponse.StatusCode >= http.StatusBadRequest {
		apiError := ParseErrorResponse(httpResponse.StatusCode, responseBytes)
		apiError.RetryAfter = parseRetryAfter(httpResponse.Header.Get(retryAfterHeader))
		return nil, apiError
	}

	return responseBytes, nil
}

func (client *Client) computeBackoff(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if retryAfter > maxRetryAfter {
			return maxRetryAfter
		}
		return retryAfter
	}

	exponential := client.backoffBase * (1 << attempt)
	if exponential > client.backoffCap || exponential <= 0 {
		exponential = client.backoffCap
	}

	jitterFactor := client.randFloat()
	if jitterFactor < 0 {
		jitterFactor = 0
	}
	if jitterFactor > 1 {
		jitterFactor = 1
	}

	return time.Duration(float64(exponential) * jitterFactor)
}

func serializeBody(body any, verbose bool) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	bodyBytes, marshalError := json.Marshal(body)
	if marshalError != nil {
		return nil, fmt.Errorf("failed to serialize request body: %w", marshalError)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "[DEBUG] Request body: %s\n", string(RedactJSONForLog(bodyBytes)))
	}

	return bodyBytes, nil
}

func buildRequestHeaders(method string, custom map[string]string, generateIdempotencyKey func() string) map[string]string {
	merged := make(map[string]string, len(custom)+1)
	for headerName, headerValue := range custom {
		if headerValue == "" {
			continue
		}
		merged[headerName] = headerValue
	}

	if _, isUnsafe := unsafeMethods[method]; !isUnsafe {
		return merged
	}

	if _, alreadySet := merged[idempotencyKeyHeader]; alreadySet {
		return merged
	}

	merged[idempotencyKeyHeader] = generateIdempotencyKey()
	return merged
}

func classifyForRetry(err error) (bool, time.Duration) {
	if errors.Is(err, context.Canceled) {
		return false, 0
	}

	var apiError *APIError
	if errors.As(err, &apiError) {
		switch {
		case apiError.StatusCode == http.StatusTooManyRequests:
			return true, apiError.RetryAfter
		case apiError.StatusCode >= serverErrorThreshold:
			return true, apiError.RetryAfter
		default:
			return false, 0
		}
	}

	return true, 0
}

func parseRetryAfter(headerValue string) time.Duration {
	trimmed := strings.TrimSpace(headerValue)
	if trimmed == "" {
		return 0
	}

	if seconds, parseError := strconv.Atoi(trimmed); parseError == nil {
		if seconds <= 0 {
			return 0
		}
		duration := time.Duration(seconds) * time.Second
		if duration > maxRetryAfter {
			return maxRetryAfter
		}
		return duration
	}

	if parsedTime, parseError := http.ParseTime(trimmed); parseError == nil {
		delta := time.Until(parsedTime)
		if delta <= 0 {
			return 0
		}
		if delta > maxRetryAfter {
			return maxRetryAfter
		}
		return delta
	}

	return 0
}

func decodeResponse(responseBody []byte, result any) error {
	if result == nil || len(responseBody) == 0 {
		return nil
	}

	if unmarshalError := json.Unmarshal(responseBody, result); unmarshalError != nil {
		return fmt.Errorf("failed to parse response JSON: %w", unmarshalError)
	}

	return nil
}

func resolveAPIKey(profileName string, activeProfile *config.Profile) (string, error) {
	keyringKey, keyringError := config.GetAPIKey(profileName)
	if keyringError == nil && keyringKey != "" {
		return keyringKey, nil
	}

	if activeProfile.APIKey != "" {
		return activeProfile.APIKey, nil
	}

	return "", fmt.Errorf("no API key found for profile %q — run 'arara login' to configure", profileName)
}

func buildUserAgent() string {
	return fmt.Sprintf("arara-cli/%s", version.Version)
}
