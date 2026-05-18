package telemetry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	httpSendTimeout   = 5 * time.Second
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
	userAgentHeader   = "User-Agent"
	defaultUserAgent  = "arara-cli-telemetry"
)

type httpSendBody struct {
	Events []Event `json:"events"`
}

// HTTPSender ships batches over HTTP to the AraraHQ telemetry endpoint.
// Stateless and safe to keep around for the lifetime of the process.
type HTTPSender struct {
	BaseURL    string
	Path       string
	HTTPClient *http.Client
	UserAgent  string
}

// NewHTTPSender builds a sender wired to the given base URL. The path
// defaults to DefaultIngestPath; callers can override for tests or to
// point at a staging endpoint.
func NewHTTPSender(baseURL string) *HTTPSender {
	return &HTTPSender{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Path:       DefaultIngestPath,
		HTTPClient: &http.Client{Timeout: httpSendTimeout},
		UserAgent:  defaultUserAgent,
	}
}

// Send POSTs one batch. Returns nil on 2xx, error on anything else. The
// caller (Recorder.Flush) decides what to do with the error — usually
// log in verbose mode and move on.
func (sender *HTTPSender) Send(events []Event) error {
	if len(events) == 0 {
		return nil
	}

	encoded, marshalErr := json.Marshal(httpSendBody{Events: events})
	if marshalErr != nil {
		return fmt.Errorf("encode telemetry batch: %w", marshalErr)
	}

	url := sender.BaseURL + sender.Path
	request, requestErr := http.NewRequest(http.MethodPost, url, bytes.NewReader(encoded))
	if requestErr != nil {
		return fmt.Errorf("build telemetry request: %w", requestErr)
	}
	request.Header.Set(contentTypeHeader, contentTypeJSON)
	request.Header.Set(userAgentHeader, sender.UserAgent)

	response, doErr := sender.HTTPClient.Do(request)
	if doErr != nil {
		return fmt.Errorf("send telemetry: %w", doErr)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	if response.StatusCode >= 300 {
		return fmt.Errorf("telemetry endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}
