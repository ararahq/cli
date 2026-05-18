package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	streamEventsPath      = "/v1/stream"
	sseEventPrefix        = "event:"
	sseDataPrefix         = "data:"
	sseContentType        = "text/event-stream"
	acceptHeader          = "Accept"
	sseInitialBackoff     = 1 * time.Second
	sseMaxBackoff         = 30 * time.Second
	sseBackoffMultiplier  = 2
	sseEventChannelBuffer = 64
)

// SSEEvent represents a single Server-Sent Event received from the stream.
//
// When the backend wraps an event in an envelope (RFC 0003), Data carries
// only the inner payload JSON (the part the customer's webhook handler
// would normally receive) and Signature carries the pre-computed
// `sha256=<hex>` HMAC. Old-format events leave Signature empty and Data
// holds the raw `data:` line as-is.
type SSEEvent struct {
	Event     string
	Data      string
	Signature string
}

// streamEnvelope is the shape backends emit when they want the CLI to
// re-sign forwarded webhooks (RFC 0003). All three keys must be present
// for the envelope to be detected — otherwise we treat `data:` as a raw
// payload string for backwards compatibility.
type streamEnvelope struct {
	Event     string          `json:"event"`
	Data      json.RawMessage `json:"data"`
	Signature string          `json:"signature"`
}

// StreamEvents connects to the SSE endpoint and returns a channel of events.
// The channel is closed when the context is cancelled or the connection is
// permanently lost. Temporary disconnections are retried with exponential backoff.
func (client *Client) StreamEvents(ctx context.Context) (<-chan SSEEvent, error) {
	eventChannel := make(chan SSEEvent, sseEventChannelBuffer)

	go client.consumeStream(ctx, eventChannel)

	return eventChannel, nil
}

func (client *Client) consumeStream(ctx context.Context, eventChannel chan<- SSEEvent) {
	defer close(eventChannel)

	backoff := sseInitialBackoff

	for {
		if ctx.Err() != nil {
			return
		}

		connectionError := client.readSSEStream(ctx, eventChannel)
		if ctx.Err() != nil {
			return
		}

		if connectionError != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff = nextBackoff(backoff)
			}
			continue
		}

		backoff = sseInitialBackoff
	}
}

func (client *Client) readSSEStream(ctx context.Context, eventChannel chan<- SSEEvent) error {
	requestURL := client.baseURL + streamEventsPath

	httpRequest, requestError := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if requestError != nil {
		return fmt.Errorf("failed to create SSE request: %w", requestError)
	}

	httpRequest.Header.Set(authorizationHeader, bearerPrefix+client.apiKey)
	httpRequest.Header.Set(acceptHeader, sseContentType)
	httpRequest.Header.Set(userAgentHeader, buildUserAgent())

	sseClient := &http.Client{}
	httpResponse, responseError := sseClient.Do(httpRequest)
	if responseError != nil {
		return fmt.Errorf("SSE connection failed: %w", responseError)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE endpoint returned status %d", httpResponse.StatusCode)
	}

	scanner := bufio.NewScanner(httpResponse.Body)
	var currentEvent SSEEvent

	for scanner.Scan() {
		if ctx.Err() != nil {
			return nil
		}

		line := scanner.Text()

		if isEmptyLine(line) {
			dispatchEvent(ctx, eventChannel, &currentEvent)
			continue
		}

		parseSSELine(line, &currentEvent)
	}

	if scanError := scanner.Err(); scanError != nil {
		return fmt.Errorf("SSE stream read error: %w", scanError)
	}

	return nil
}

func parseSSELine(line string, currentEvent *SSEEvent) {
	if strings.HasPrefix(line, sseEventPrefix) {
		currentEvent.Event = strings.TrimSpace(strings.TrimPrefix(line, sseEventPrefix))
		return
	}

	if strings.HasPrefix(line, sseDataPrefix) {
		currentEvent.Data = strings.TrimSpace(strings.TrimPrefix(line, sseDataPrefix))
		return
	}
}

func dispatchEvent(ctx context.Context, eventChannel chan<- SSEEvent, currentEvent *SSEEvent) {
	if currentEvent.Data == "" && currentEvent.Event == "" {
		return
	}

	unwrapEnvelopeInto(currentEvent)

	select {
	case eventChannel <- *currentEvent:
	case <-ctx.Done():
	}

	currentEvent.Event = ""
	currentEvent.Data = ""
	currentEvent.Signature = ""
}

// unwrapEnvelopeInto checks whether currentEvent.Data carries an RFC 0003
// envelope and, if so, replaces Data with the inner payload and lifts the
// signature into the dedicated field. Non-envelope data is left untouched
// so legacy backends keep working.
func unwrapEnvelopeInto(currentEvent *SSEEvent) {
	trimmed := strings.TrimSpace(currentEvent.Data)
	if !looksLikeJSONObject(trimmed) {
		return
	}

	var envelope streamEnvelope
	if unmarshalErr := json.Unmarshal([]byte(trimmed), &envelope); unmarshalErr != nil {
		return
	}
	if envelope.Signature == "" || len(envelope.Data) == 0 {
		return
	}

	if envelope.Event != "" && currentEvent.Event == "" {
		currentEvent.Event = envelope.Event
	}
	currentEvent.Data = string(envelope.Data)
	currentEvent.Signature = envelope.Signature
}

func looksLikeJSONObject(s string) bool {
	return len(s) >= 2 && s[0] == '{' && s[len(s)-1] == '}'
}

func isEmptyLine(line string) bool {
	return strings.TrimSpace(line) == ""
}

func nextBackoff(current time.Duration) time.Duration {
	next := current * sseBackoffMultiplier
	if next > sseMaxBackoff {
		return sseMaxBackoff
	}
	return next
}
