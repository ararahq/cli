package api

import (
	"encoding/json"
	"testing"
)

func TestNormalizeReceiver(t *testing.T) {
	cases := map[string]string{
		"+5511999999999":          "whatsapp:+5511999999999",
		"5511999999999":           "whatsapp:+5511999999999",
		"whatsapp:+5511999999999": "whatsapp:+5511999999999",
		"whatsapp:5511999999999":  "whatsapp:+5511999999999",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := normalizeReceiver(input); got != want {
				t.Errorf("normalizeReceiver(%q): want %q, got %q", input, want, got)
			}
		})
	}
}

func TestEnsurePhonePrefix(t *testing.T) {
	if got := ensurePhonePrefix("5511"); got != "+5511" {
		t.Errorf("want +5511, got %q", got)
	}
	if got := ensurePhonePrefix("+5511"); got != "+5511" {
		t.Errorf("want +5511, got %q", got)
	}
}

func TestMessageResponse_UnmarshalNullIDAsEmpty(t *testing.T) {
	body := `{"id":null,"status":"queued","receiver":"+5511","mode":"test"}`
	var response MessageResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("nullable id should not fail decode: %v", err)
	}
	if response.ID != "" {
		t.Errorf("null id should map to empty string, got %q", response.ID)
	}
	if response.Status != "queued" {
		t.Errorf("status decoded wrong: %q", response.Status)
	}
}

func TestMessageResponse_UnmarshalNonNullID(t *testing.T) {
	body := `{"id":"msg_abc","status":"queued","receiver":"+5511","mode":"live"}`
	var response MessageResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if response.ID != "msg_abc" {
		t.Errorf("id: want msg_abc, got %q", response.ID)
	}
}

func TestMessageResponse_UnmarshalCostField(t *testing.T) {
	body := `{"id":"msg_x","status":"sent","receiver":"+1","mode":"test","cost":0.05}`
	var response MessageResponse
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatal(err)
	}
	if response.Cost != 0.05 {
		t.Errorf("cost: want 0.05, got %v", response.Cost)
	}
}
