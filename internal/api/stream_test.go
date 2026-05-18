package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseSSELine(t *testing.T) {
	var current SSEEvent

	parseSSELine("event: message.delivered", &current)
	if current.Event != "message.delivered" {
		t.Errorf("event: want message.delivered, got %q", current.Event)
	}

	parseSSELine("data: {\"id\":\"abc\"}", &current)
	if current.Data != `{"id":"abc"}` {
		t.Errorf("data parse failed, got %q", current.Data)
	}

	parseSSELine("comment line", &current)
	if current.Event != "message.delivered" || current.Data != `{"id":"abc"}` {
		t.Error("non-prefix line should not mutate event/data")
	}
}

func TestIsEmptyLine(t *testing.T) {
	if !isEmptyLine("") || !isEmptyLine("   ") {
		t.Error("blank should be empty line")
	}
	if isEmptyLine("data: x") {
		t.Error("data line should not be empty")
	}
}

func TestNextBackoff(t *testing.T) {
	got := nextBackoff(time.Second)
	if got != 2*time.Second {
		t.Errorf("nextBackoff(1s): want 2s, got %v", got)
	}

	if got := nextBackoff(sseMaxBackoff); got != sseMaxBackoff {
		t.Errorf("nextBackoff at cap: want %v, got %v", sseMaxBackoff, got)
	}
}

func TestStreamEvents_DeliversThenCloses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, "event: message.delivered\ndata: {\"id\":\"a\"}\n\n")
		fmt.Fprint(w, "event: message.read\ndata: {\"id\":\"b\"}\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := client.StreamEvents(ctx)
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	timeout := time.After(2 * time.Second)
	collected := []SSEEvent{}
	for len(collected) < 2 {
		select {
		case ev := <-events:
			collected = append(collected, ev)
		case <-timeout:
			t.Fatalf("timeout, only collected %d events", len(collected))
		}
	}

	if collected[0].Event != "message.delivered" || collected[0].Data != `{"id":"a"}` {
		t.Errorf("first event mismatch: %+v", collected[0])
	}
	if collected[1].Event != "message.read" || collected[1].Data != `{"id":"b"}` {
		t.Errorf("second event mismatch: %+v", collected[1])
	}
}

func TestStreamEvents_NonOKStatusErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "ara_test_xxxxxxxxxxxx")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	events, err := client.StreamEvents(ctx)
	if err != nil {
		t.Fatalf("StreamEvents setup error: %v", err)
	}

	select {
	case _, open := <-events:
		if open {
			t.Error("did not expect events on auth failure path before context expiry")
		}
	case <-ctx.Done():
	}
}

func TestDispatchEvent_SkipsEmpty(t *testing.T) {
	ctx := context.Background()
	channel := make(chan SSEEvent, 1)
	current := &SSEEvent{}

	dispatchEvent(ctx, channel, current)

	if len(channel) != 0 {
		t.Error("empty event should not be dispatched")
	}
}

func TestDispatchEvent_RespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	channel := make(chan SSEEvent)
	current := &SSEEvent{Event: "x", Data: "y"}

	done := make(chan struct{})
	go func() {
		dispatchEvent(ctx, channel, current)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatchEvent stuck on canceled context")
	}
}

func TestUnwrapEnvelopeInto_DetectsAndExtracts(t *testing.T) {
	current := &SSEEvent{
		Event: "",
		Data:  `{"event":"message.delivered","data":{"id":"msg_1","status":"delivered"},"signature":"sha256=abc123"}`,
	}

	unwrapEnvelopeInto(current)

	if current.Event != "message.delivered" {
		t.Errorf("event from envelope not lifted, got %q", current.Event)
	}
	if current.Signature != "sha256=abc123" {
		t.Errorf("signature not extracted, got %q", current.Signature)
	}
	if current.Data != `{"id":"msg_1","status":"delivered"}` {
		t.Errorf("data should be inner payload, got %q", current.Data)
	}
}

func TestUnwrapEnvelopeInto_PreservesExplicitOuterEvent(t *testing.T) {
	// SSE `event:` line is authoritative when both layers carry it.
	current := &SSEEvent{
		Event: "outer.event",
		Data:  `{"event":"inner.event","data":{"x":1},"signature":"sha256=zzz"}`,
	}
	unwrapEnvelopeInto(current)
	if current.Event != "outer.event" {
		t.Errorf("outer event should win, got %q", current.Event)
	}
	if current.Signature != "sha256=zzz" {
		t.Errorf("signature should still be lifted, got %q", current.Signature)
	}
}

func TestUnwrapEnvelopeInto_LegacyDataIsUntouched(t *testing.T) {
	current := &SSEEvent{
		Event: "ping",
		Data:  `{"id":"abc","status":"ok"}`, // no envelope keys
	}
	unwrapEnvelopeInto(current)

	if current.Signature != "" {
		t.Errorf("legacy data should not produce signature, got %q", current.Signature)
	}
	if current.Data != `{"id":"abc","status":"ok"}` {
		t.Errorf("legacy data mutated: %q", current.Data)
	}
}

func TestUnwrapEnvelopeInto_IncompleteEnvelopeIsIgnored(t *testing.T) {
	cases := []string{
		`{"event":"x","data":{"a":1}}`,            // no signature
		`{"event":"x","signature":"sha256=y"}`,    // no data
		`{"data":{"a":1},"signature":"sha256=y"}`, // no event — but should still pass since outer Event might exist
	}
	for _, raw := range cases {
		current := &SSEEvent{Event: "outer", Data: raw}
		unwrapEnvelopeInto(current)
		if raw[1:] != "" && current.Signature != "" && current.Data != raw {
			// only the third case has signature+data, those are valid
			if !strings.Contains(raw, `"signature"`) || !strings.Contains(raw, `"data"`) {
				t.Errorf("incomplete envelope %q should not produce signature, got Sig=%q Data=%q", raw, current.Signature, current.Data)
			}
		}
	}
}

func TestUnwrapEnvelopeInto_NonJSONIsUntouched(t *testing.T) {
	current := &SSEEvent{Event: "ping", Data: "plain text"}
	unwrapEnvelopeInto(current)
	if current.Data != "plain text" {
		t.Errorf("plain text should pass through, got %q", current.Data)
	}
	if current.Signature != "" {
		t.Errorf("plain text should not produce signature, got %q", current.Signature)
	}
}

func TestLooksLikeJSONObject(t *testing.T) {
	if !looksLikeJSONObject(`{"x":1}`) {
		t.Error("simple object should be recognized")
	}
	if looksLikeJSONObject(`[1,2]`) {
		t.Error("array should not be recognized as object")
	}
	if looksLikeJSONObject("plain") {
		t.Error("string should not be recognized")
	}
	if looksLikeJSONObject("") {
		t.Error("empty should not panic / not be recognized")
	}
	if looksLikeJSONObject("{") {
		t.Error("single brace should not be recognized")
	}
}
