package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	webhookListenersPath           = "/v1/cli/webhook-listeners"
	webhookListenerHeartbeatSuffix = "/heartbeat"
)

// ErrCaptureNotSupported is returned when the backend doesn't yet expose
// the capture-listener endpoints. The CLI uses it to fall back to passive
// SSE mode without breaking the user flow.
var ErrCaptureNotSupported = errors.New("backend does not support webhook capture listeners (RFC 0003)")

// CaptureConflictError is returned when another CLI session is already
// holding the org's capture slot. The CLI displays the conflict details
// (owner, expiresAt) so the user can decide whether to wait or kill the
// other session.
type CaptureConflictError struct {
	ListenerID string
	Owner      string
	StartedAt  string
	ExpiresAt  string
	Message    string
}

func (err *CaptureConflictError) Error() string {
	return fmt.Sprintf("capture conflict: another CLI session %s (owner=%q, expires=%s) is active", err.ListenerID, err.Owner, err.ExpiresAt)
}

// CreateWebhookListenerRequest is the body of POST /v1/cli/webhook-listeners.
// Both fields are optional — the backend supplies sensible defaults when
// they're empty.
type CreateWebhookListenerRequest struct {
	Owner      string `json:"owner,omitempty"`
	TTLSeconds int    `json:"ttlSeconds,omitempty"`
}

// CreateWebhookListenerResponse is the 201 body. Secret is shown ONCE and
// must not be persisted to disk by the CLI — it dies with the process.
type CreateWebhookListenerResponse struct {
	ID                       string `json:"id"`
	Secret                   string `json:"secret"`
	OrganizationID           string `json:"organizationId"`
	ExpiresAt                string `json:"expiresAt"`
	HeartbeatIntervalSeconds int    `json:"heartbeatIntervalSeconds"`
}

// CreateWebhookListener registers an ephemeral capture session. Returns
// ErrCaptureNotSupported on 404 (graceful fallback) and CaptureConflictError
// on 409 (another session holds the slot).
func (client *Client) CreateWebhookListener(req CreateWebhookListenerRequest) (*CreateWebhookListenerResponse, error) {
	var response CreateWebhookListenerResponse
	if postErr := client.Post(webhookListenersPath, req, &response); postErr != nil {
		return nil, classifyCaptureError(postErr)
	}
	return &response, nil
}

// HeartbeatWebhookListener extends the listener's TTL. Returns nil on 204
// success. On 410 (already expired), returns a sentinel so the CLI can
// surface "listener expired, re-run with --capture to grab a fresh slot".
func (client *Client) HeartbeatWebhookListener(listenerID string) error {
	path := webhookListenersPath + "/" + listenerID + webhookListenerHeartbeatSuffix
	if postErr := client.Post(path, struct{}{}, nil); postErr != nil {
		return classifyCaptureError(postErr)
	}
	return nil
}

// ReleaseWebhookListener tells the backend the CLI is done. Idempotent on
// the backend side. Best-effort from the CLI's perspective: failure here
// just means the listener will expire naturally on its TTL.
func (client *Client) ReleaseWebhookListener(listenerID string) error {
	path := webhookListenersPath + "/" + listenerID
	if delErr := client.Delete(path); delErr != nil {
		return classifyCaptureError(delErr)
	}
	return nil
}

// classifyCaptureError unwraps an APIError and returns one of the
// well-known sentinel errors when applicable. Anything else passes through.
func classifyCaptureError(err error) error {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return err
	}

	if apiError.StatusCode == http.StatusNotFound && apiError.Code == "" {
		// Spring's default "no controller mapped" page — feature isn't
		// deployed. Domain 404s with a structured code are passed through.
		return ErrCaptureNotSupported
	}

	// EqualFold porque o backend emite CAPTURE_LISTENER_ACTIVE em maiusculo
	// (GlobalExceptionHandler.kt) e a comparacao exata em minusculo nunca casava: o usuario via
	// um conflito generico em vez de quem esta com o listener e ate quando.
	if apiError.StatusCode == http.StatusConflict &&
		strings.EqualFold(apiError.Code, "CAPTURE_LISTENER_ACTIVE") {
		return &CaptureConflictError{
			ListenerID: stringFromDetails(apiError, "listenerId"),
			Owner:      stringFromDetails(apiError, "owner"),
			StartedAt:  stringFromDetails(apiError, "startedAt"),
			ExpiresAt:  stringFromDetails(apiError, "expiresAt"),
			Message:    apiError.Message,
		}
	}

	return err
}

// stringFromDetails reads the apiError's RawBody for the named key under
// `error.details`. Defensive — APIError today doesn't expose a typed
// details map, so we fall back to a small JSON walk only when needed.
func stringFromDetails(apiError *APIError, key string) string {
	body := apiError.RawBody
	if body == "" {
		return ""
	}
	// The body shape is `{"error":{"code":..., "details":{<key>:<value>,...}}}`.
	// We don't bring in a real JSON tree here just for an error helper.
	// A naive substring match is robust enough — the keys are known short
	// identifiers (listenerId, owner, expiresAt) that don't appear in the
	// other fields.
	needle := `"` + key + `":"`
	start := indexOf(body, needle)
	if start < 0 {
		return ""
	}
	start += len(needle)
	end := indexOf(body[start:], `"`)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

func indexOf(haystack, needle string) int {
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if haystack[index:index+len(needle)] == needle {
			return index
		}
	}
	return -1
}
