package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/api"
	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/tui"
)

const (
	forwardTimeout         = 10 * time.Second
	forwardEventHeader     = "X-Arara-Webhook-Event"
	forwardSourceHeader    = "X-Arara-Forwarded-By"
	forwardSignatureHeader = "X-Arara-Signature"
	forwardSourceName      = "arara-cli"
	contentTypeHeaderKey   = "Content-Type"
	contentTypeJSONValue   = "application/json"
)

var (
	listenForwardToFlag string
	listenEventsFlag    string
	listenCaptureFlag   bool
)

const (
	captureHeartbeatFloorSeconds = 10
	captureHeartbeatDefault      = 30 * time.Second
	captureMaxHeartbeatFailures  = 3
	captureOwnerHostnameFallback = "unknown-host"
)

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen to live webhook events via SSE",
	Long: "Connect to the AraraHQ SSE stream and display webhook events in real time.\n" +
		"Optionally forward each event to a local URL for testing integrations.\n" +
		"Use -o stream-json to emit one NDJSON record per event for piping to tools like jq.",
	RunE: runListen,
}

func init() {
	listenCmd.Flags().StringVar(&listenForwardToFlag, "forward-to", "", "URL to forward each webhook event to via POST")
	listenCmd.Flags().StringVar(&listenEventsFlag, "events", "", "comma-separated event types to filter (e.g., message.delivered,message.read)")
	listenCmd.Flags().BoolVar(&listenCaptureFlag, "capture", false, "register an ephemeral capture listener (RFC 0003) so production webhooks are intercepted and signed for the local handler — falls back to passive SSE if backend doesn't support it")

	rootCmd.AddCommand(listenCmd)
}

func runListen(command *cobra.Command, arguments []string) error {
	if validationErr := validateForwardURL(listenForwardToFlag); validationErr != nil {
		output.PrintError(validationErr.Error())
		return validationErr
	}

	client, clientError := newClientForCmd()
	if clientError != nil {
		output.PrintError(fmt.Sprintf("Failed to create API client: %s", clientError.Error()))
		return clientError
	}

	client.SetVerbose(IsVerbose())
	return dispatchListenMode(client, parseEventFilters(listenEventsFlag), listenForwardToFlag)
}

// dispatchListenMode picks which listener implementation runs based on the
// flag combination: capture takes precedence; explicit stream-json / a
// forward URL forces non-TUI mode; everything else opens the bubbletea
// webhook viewer.
func dispatchListenMode(client *api.Client, eventFilters []string, forwardURL string) error {
	if listenCaptureFlag {
		captureCtx, captureCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer captureCancel()
		return runListenCapture(captureCtx, client, eventFilters, forwardURL)
	}

	if GetOutputFormat() == output.FormatStreamJSON || forwardURL != "" {
		return runListenStream(client, eventFilters, forwardURL)
	}

	return runListenViewer(client, eventFilters, forwardURL)
}

func runListenViewer(client *api.Client, eventFilters []string, forwardURL string) error {
	viewerModel := tui.NewWebhookViewer(tui.WebhookViewerConfig{
		Client:       client,
		ForwardToURL: forwardURL,
		EventFilters: eventFilters,
	})

	program := tea.NewProgram(viewerModel, tea.WithAltScreen())
	if _, runError := program.Run(); runError != nil {
		output.PrintError(fmt.Sprintf("Webhook viewer failed: %s", runError.Error()))
		return runError
	}
	return nil
}

// runListenCapture orchestrates the --capture lifecycle: register listener,
// print signing secret, start heartbeat goroutine, run the SSE consumer,
// release on exit. Falls back to passive runListenStream when the backend
// doesn't expose the capture endpoints (RFC 0003 not deployed).
func runListenCapture(ctx context.Context, client *api.Client, eventFilters []string, forwardURL string) error {
	created, captureErr := registerCaptureListener(client)
	if captureErr != nil {
		return handleCaptureSetupError(ctx, client, eventFilters, forwardURL, captureErr)
	}

	printCaptureBanner(created, forwardURL)

	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	defer stopHeartbeat()
	go runCaptureHeartbeat(heartbeatCtx, client, created)

	defer releaseCaptureListener(client, created.ID)

	return runListenStreamWithCtx(ctx, client, eventFilters, forwardURL)
}

func registerCaptureListener(client *api.Client) (*api.CreateWebhookListenerResponse, error) {
	return client.CreateWebhookListener(api.CreateWebhookListenerRequest{
		Owner: captureOwnerIdentity(),
	})
}

// handleCaptureSetupError centralizes the three failure modes of the
// listener-registration step: feature missing (fall back to passive SSE),
// already-active-on-other-CLI (show conflict, exit), or anything else
// (surface and exit).
func handleCaptureSetupError(ctx context.Context, client *api.Client, eventFilters []string, forwardURL string, captureErr error) error {
	if errors.Is(captureErr, api.ErrCaptureNotSupported) {
		output.PrintWarning("Capture mode not yet supported by this AraraHQ environment.")
		output.PrintWarning("Falling back to passive SSE mode — production webhooks will continue receiving callbacks too.")
		return runListenStreamWithCtx(ctx, client, eventFilters, forwardURL)
	}
	var conflict *api.CaptureConflictError
	if errors.As(captureErr, &conflict) {
		printCaptureConflict(conflict)
		return captureErr
	}
	output.PrintError(fmt.Sprintf("Failed to start capture listener: %s", captureErr.Error()))
	return captureErr
}

func releaseCaptureListener(client *api.Client, listenerID string) {
	if releaseErr := client.ReleaseWebhookListener(listenerID); releaseErr != nil {
		if IsVerbose() {
			fmt.Fprintf(os.Stderr, "[capture] release failed: %v\n", releaseErr)
		}
		return
	}
	output.PrintSuccess("Listener released. Webhooks resume normal delivery to the configured URL.")
}

// runListenStreamWithCtx is the ctx-aware twin of runListenStream. We need
// it because runListenCapture builds its own signal context and wraps the
// stream + heartbeat under the same lifetime.
func runListenStreamWithCtx(ctx context.Context, client *api.Client, eventFilters []string, forwardURL string) error {
	events, streamError := client.StreamEvents(ctx)
	if streamError != nil {
		return fmt.Errorf("failed to start SSE stream: %w", streamError)
	}

	forwarder := newForwarder(forwardURL)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, channelOpen := <-events:
			if !channelOpen {
				return nil
			}
			if !eventMatchesFilter(event.Event, eventFilters) {
				continue
			}
			if writeError := writeStreamEvent(os.Stdout, event); writeError != nil {
				return writeError
			}
			if forwarder != nil {
				forwarder.send(event)
			}
		}
	}
}

func runCaptureHeartbeat(ctx context.Context, client *api.Client, listener *api.CreateWebhookListenerResponse) {
	interval := captureHeartbeatDefault
	if listener.HeartbeatIntervalSeconds >= captureHeartbeatFloorSeconds {
		interval = time.Duration(listener.HeartbeatIntervalSeconds) * time.Second
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	consecutiveFailures := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if hbErr := client.HeartbeatWebhookListener(listener.ID); hbErr != nil {
				consecutiveFailures++
				if IsVerbose() {
					fmt.Fprintf(os.Stderr, "[capture] heartbeat failed (%d/%d): %v\n", consecutiveFailures, captureMaxHeartbeatFailures, hbErr)
				}
				if consecutiveFailures >= captureMaxHeartbeatFailures {
					output.PrintWarning("Capture listener heartbeat failed 3× — listener may have expired. Re-run with --capture to grab a fresh slot.")
					return
				}
				continue
			}
			consecutiveFailures = 0
		}
	}
}

func printCaptureBanner(listener *api.CreateWebhookListenerResponse, forwardURL string) {
	output.PrintSuccess(fmt.Sprintf("Capture listener active (id: %s, expires: %s)", listener.ID, listener.ExpiresAt))
	output.PrintInfo(fmt.Sprintf("Signing secret: %s", listener.Secret))
	output.PrintInfo("Use this secret in your local handler to verify X-Arara-Signature.")
	if forwardURL != "" {
		output.PrintInfo(fmt.Sprintf("Forwarding all events to %s", forwardURL))
	} else {
		output.PrintWarning("No --forward-to set — events will print to stdout but won't be POSTed anywhere.")
	}
}

func printCaptureConflict(conflict *api.CaptureConflictError) {
	output.PrintError("Another CLI session is already capturing for this org:")
	fmt.Fprintf(os.Stderr, "    listener:  %s\n", conflict.ListenerID)
	if conflict.Owner != "" {
		fmt.Fprintf(os.Stderr, "    owner:     %s\n", conflict.Owner)
	}
	if conflict.StartedAt != "" {
		fmt.Fprintf(os.Stderr, "    started:   %s\n", conflict.StartedAt)
	}
	if conflict.ExpiresAt != "" {
		fmt.Fprintf(os.Stderr, "    expires:   %s\n", conflict.ExpiresAt)
	}
	output.PrintInfo("Stop that session before starting a new one, or wait until it expires.")
}

// validateForwardURL rejects --forward-to values that aren't http/https URLs
// pointing at a host. Empty is allowed (means "don't forward"). Anything
// else fails fast before we open the SSE connection so the user gets a
// clear, immediate error instead of a stream of failed POSTs to garbage.
func validateForwardURL(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	parsed, parseErr := url.Parse(trimmed)
	if parseErr != nil {
		return fmt.Errorf("--forward-to %q is not a valid URL: %w", raw, parseErr)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("--forward-to %q must use http or https scheme (got %q)", raw, parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("--forward-to %q is missing a host", raw)
	}
	return nil
}

// captureOwnerIdentity builds the `owner` string sent to the backend so a
// human looking at conflict errors can tell which workstation holds the
// active capture slot. Format: `<goos>/<goarch>@<hostname>`.
func captureOwnerIdentity() string {
	hostname, hostErr := os.Hostname()
	if hostErr != nil || hostname == "" {
		hostname = captureOwnerHostnameFallback
	}
	return fmt.Sprintf("%s/%s@%s", runtime.GOOS, runtime.GOARCH, hostname)
}

func runListenStream(client *api.Client, eventFilters []string, forwardURL string) error {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	events, streamError := client.StreamEvents(ctx)
	if streamError != nil {
		return fmt.Errorf("failed to start SSE stream: %w", streamError)
	}

	forwarder := newForwarder(forwardURL)

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, channelOpen := <-events:
			if !channelOpen {
				return nil
			}
			if !eventMatchesFilter(event.Event, eventFilters) {
				continue
			}
			if writeError := writeStreamEvent(os.Stdout, event); writeError != nil {
				return writeError
			}
			if forwarder != nil {
				forwarder.send(event)
			}
		}
	}
}

// streamForwarder POSTs each SSE event to a local URL while the listen loop
// is running. Failures are reported on stderr but never abort the loop —
// dropping a forward POST should not silence the live stream.
type streamForwarder struct {
	url        string
	httpClient *http.Client
	stderr     io.Writer
}

func newForwarder(forwardURL string) *streamForwarder {
	if forwardURL == "" {
		return nil
	}
	return &streamForwarder{
		url:        forwardURL,
		httpClient: &http.Client{Timeout: forwardTimeout},
		stderr:     os.Stderr,
	}
}

func (forwarder *streamForwarder) send(event api.SSEEvent) {
	if forwarder == nil {
		return
	}

	requestBody := bytes.NewBufferString(event.Data)
	httpRequest, requestError := http.NewRequest(http.MethodPost, forwarder.url, requestBody)
	if requestError != nil {
		fmt.Fprintf(forwarder.stderr, "forward to %s failed: %v\n", forwarder.url, requestError)
		return
	}

	httpRequest.Header.Set(contentTypeHeaderKey, contentTypeJSONValue)
	httpRequest.Header.Set(forwardEventHeader, event.Event)
	httpRequest.Header.Set(forwardSourceHeader, forwardSourceName)
	if event.Signature != "" {
		// Backend pre-computed the HMAC against the capture session secret
		// (RFC 0003). Propagating verbatim lets the local handler use the
		// same verification logic it'll use against the production secret.
		httpRequest.Header.Set(forwardSignatureHeader, event.Signature)
	}

	startTime := time.Now()
	httpResponse, responseError := forwarder.httpClient.Do(httpRequest)
	if responseError != nil {
		fmt.Fprintf(forwarder.stderr, "forward to %s failed: %v\n", forwarder.url, responseError)
		return
	}
	defer httpResponse.Body.Close()
	_, _ = io.Copy(io.Discard, httpResponse.Body)

	fmt.Fprintf(forwarder.stderr, "forwarded %s → %s [%d in %s]\n", event.Event, forwarder.url, httpResponse.StatusCode, time.Since(startTime).Round(time.Millisecond))
}

func writeStreamEvent(writer interface {
	Write([]byte) (int, error)
}, event api.SSEEvent,
) error {
	record := map[string]any{
		"event": event.Event,
		"data":  decodeEventData(event.Data),
	}
	return output.WriteJSONLine(writer, record)
}

func decodeEventData(raw string) any {
	if raw == "" {
		return nil
	}
	var parsed any
	if unmarshalError := json.Unmarshal([]byte(raw), &parsed); unmarshalError == nil {
		return parsed
	}
	return raw
}

func eventMatchesFilter(eventName string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	for _, filter := range filters {
		if strings.EqualFold(filter, eventName) {
			return true
		}
	}
	return false
}

func parseEventFilters(rawFilters string) []string {
	if rawFilters == "" {
		return nil
	}

	parts := strings.Split(rawFilters, ",")
	filters := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			filters = append(filters, trimmed)
		}
	}

	return filters
}
