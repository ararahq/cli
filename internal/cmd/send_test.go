package cmd

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseCommaSeparatedVars(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", []string{}},
		{"João", []string{"João"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b ,c ,, ,", []string{"a", "b", "c"}},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := parseCommaSeparatedVars(tc.input)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseCommaSeparatedVars(%q): want %#v, got %#v", tc.input, tc.want, got)
			}
		})
	}
}

func TestExtractPhoneNumber(t *testing.T) {
	cases := map[string]string{
		"whatsapp:+5511": "+5511",
		"+5511":          "+5511",
		"":               "",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := extractPhoneNumber(input); got != want {
				t.Errorf("extractPhoneNumber(%q): want %q, got %q", input, want, got)
			}
		})
	}
}

func TestValidateSendFlags(t *testing.T) {
	type fixture struct {
		name     string
		to       string
		template string
		body     string
		dryRun   bool
		watch    bool
		wantErr  string
	}
	cases := []fixture{
		{name: "missing to", template: "hello", wantErr: "--to is required"},
		{name: "missing template and body", to: "+5511", wantErr: "either --template or --body"},
		{name: "both template and body", to: "+5511", template: "hello", body: "hi", wantErr: "mutually exclusive"},
		{name: "valid template", to: "+5511", template: "hello"},
		{name: "valid body", to: "+5511", body: "hi"},
		{name: "dry-run + watch", to: "+5511", template: "hello", dryRun: true, watch: true, wantErr: "--dry-run and --watch are mutually exclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			restoreSendFlags(t)
			sendToFlag = tc.to
			sendTemplateFlag = tc.template
			sendBodyFlag = tc.body
			sendDryRunFlag = tc.dryRun
			sendWatchFlag = tc.watch

			err := validateSendFlags()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

func TestIsTerminalMessageStatus(t *testing.T) {
	terminal := []string{"delivered", "Read", " FAILED ", "undelivered", "sent", "canceled"}
	for _, status := range terminal {
		if !isTerminalMessageStatus(status) {
			t.Errorf("status %q should be terminal", status)
		}
	}
	nonTerminal := []string{"queued", "accepted", "scheduled", "", "pending"}
	for _, status := range nonTerminal {
		if isTerminalMessageStatus(status) {
			t.Errorf("status %q should NOT be terminal", status)
		}
	}
}

func TestIsFailureMessageStatus(t *testing.T) {
	failures := []string{"failed", "UNDELIVERED", "canceled"}
	for _, status := range failures {
		if !isFailureMessageStatus(status) {
			t.Errorf("status %q should be failure", status)
		}
	}
	nonFailures := []string{"delivered", "read", "sent", "queued"}
	for _, status := range nonFailures {
		if isFailureMessageStatus(status) {
			t.Errorf("status %q should NOT be failure", status)
		}
	}
}

func TestNextWatchInterval(t *testing.T) {
	got := nextWatchInterval(watchInitialPollInterval)
	if got < watchInitialPollInterval*2 || got > watchInitialPollInterval*2+watchInitialPollInterval/3 {
		t.Errorf("interval should ~double + jitter, got %v", got)
	}

	got = nextWatchInterval(watchMaxPollInterval)
	if got < watchMaxPollInterval || got > watchMaxPollInterval+watchMaxPollInterval/3 {
		t.Errorf("interval should be capped near watchMaxPollInterval, got %v", got)
	}

	got = nextWatchInterval(time.Hour)
	if got < watchMaxPollInterval || got > watchMaxPollInterval+watchMaxPollInterval/3 {
		t.Errorf("very large input should clamp to cap, got %v", got)
	}
}

func TestNoSendFlagsProvided(t *testing.T) {
	restoreSendFlags(t)
	if !noSendFlagsProvided() {
		t.Error("expected true when all flags empty")
	}

	restoreSendFlags(t)
	sendToFlag = "+5511"
	if noSendFlagsProvided() {
		t.Error("expected false when --to provided")
	}
}

func TestBuildSendMessageRequest(t *testing.T) {
	restoreSendFlags(t)
	sendToFlag = "+5511"
	sendTemplateFlag = "hello"
	sendVarsFlag = "João, Doe ,"
	sendScheduledAtFlag = "2026-01-01T00:00:00Z"

	got := buildSendMessageRequest()

	if got.Receiver != "+5511" {
		t.Errorf("Receiver: want +5511, got %q", got.Receiver)
	}
	if got.TemplateName != "hello" {
		t.Errorf("TemplateName: want hello, got %q", got.TemplateName)
	}
	if !reflect.DeepEqual(got.TemplateVariables, []string{"João", "Doe"}) {
		t.Errorf("TemplateVariables wrong: %#v", got.TemplateVariables)
	}
	if got.ScheduledAt != "2026-01-01T00:00:00Z" {
		t.Errorf("ScheduledAt mismatch: %q", got.ScheduledAt)
	}
}

func TestBuildSendMessageRequest_NoVars(t *testing.T) {
	restoreSendFlags(t)
	sendToFlag = "+5511"
	sendBodyFlag = "hi"

	got := buildSendMessageRequest()
	if got.TemplateVariables != nil {
		t.Errorf("TemplateVariables should be nil when --vars empty, got %#v", got.TemplateVariables)
	}
}

func restoreSendFlags(t *testing.T) {
	t.Helper()
	originalTo := sendToFlag
	originalTemplate := sendTemplateFlag
	originalBody := sendBodyFlag
	originalVars := sendVarsFlag
	originalScheduled := sendScheduledAtFlag
	originalDryRun := sendDryRunFlag
	originalWatch := sendWatchFlag
	originalWatchTimeout := sendWatchTimeoutFlag
	t.Cleanup(func() {
		sendToFlag = originalTo
		sendTemplateFlag = originalTemplate
		sendBodyFlag = originalBody
		sendVarsFlag = originalVars
		sendScheduledAtFlag = originalScheduled
		sendDryRunFlag = originalDryRun
		sendWatchFlag = originalWatch
		sendWatchTimeoutFlag = originalWatchTimeout
	})
	sendToFlag = ""
	sendTemplateFlag = ""
	sendBodyFlag = ""
	sendVarsFlag = ""
	sendScheduledAtFlag = ""
	sendDryRunFlag = false
	sendWatchFlag = false
	sendWatchTimeoutFlag = defaultWatchTimeout
}
