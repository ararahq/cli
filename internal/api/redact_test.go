package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactJSONForLog_TopLevelSensitiveKey(t *testing.T) {
	in := []byte(`{"accessToken":"secret-abc","name":"alice"}`)
	out := decodeRedacted(t, in)

	if got := out["accessToken"]; got != redactedPlaceholder {
		t.Errorf("accessToken not redacted: %v", got)
	}
	if got := out["name"]; got != "alice" {
		t.Errorf("non-sensitive value mutated: %v", got)
	}
}

func TestRedactJSONForLog_NestedObject(t *testing.T) {
	in := []byte(`{"user":{"id":"u1","apiKey":"ak_live_xxx"}}`)
	out := decodeRedacted(t, in)

	user, ok := out["user"].(map[string]any)
	if !ok {
		t.Fatalf("user not preserved as object: %T", out["user"])
	}
	if user["apiKey"] != redactedPlaceholder {
		t.Errorf("nested apiKey not redacted: %v", user["apiKey"])
	}
	if user["id"] != "u1" {
		t.Errorf("nested non-sensitive id mutated: %v", user["id"])
	}
}

func TestRedactJSONForLog_ArrayOfObjects(t *testing.T) {
	in := []byte(`{"sessions":[{"token":"t1"},{"token":"t2","label":"laptop"}]}`)
	out := decodeRedacted(t, in)

	sessions, ok := out["sessions"].([]any)
	if !ok || len(sessions) != 2 {
		t.Fatalf("sessions array missing or wrong length: %T %v", out["sessions"], out["sessions"])
	}
	for index, raw := range sessions {
		session, isMap := raw.(map[string]any)
		if !isMap {
			t.Fatalf("session[%d] is not an object: %T", index, raw)
		}
		if session["token"] != redactedPlaceholder {
			t.Errorf("session[%d].token not redacted: %v", index, session["token"])
		}
	}
	if label := sessions[1].(map[string]any)["label"]; label != "laptop" {
		t.Errorf("non-sensitive label mutated: %v", label)
	}
}

func TestRedactJSONForLog_MixedCaseAndUnderscoreKeys(t *testing.T) {
	in := []byte(`{"Refresh_Token":"r1","Access_Token":"a1","API_KEY":"k1","Authorization":"Bearer x"}`)
	out := decodeRedacted(t, in)

	for _, key := range []string{"Refresh_Token", "Access_Token", "API_KEY", "Authorization"} {
		if got := out[key]; got != redactedPlaceholder {
			t.Errorf("%s not redacted (got %v)", key, got)
		}
	}
}

func TestRedactJSONForLog_NonJSONInput(t *testing.T) {
	in := []byte("ara_live_thisIsAnOpaqueToken1234567890")
	out := RedactJSONForLog(in)

	if !strings.HasPrefix(string(out), "<non-json body redacted") {
		t.Errorf("non-json input should produce placeholder, got: %s", string(out))
	}
	if strings.Contains(string(out), "ara_live_") {
		t.Errorf("token leaked into placeholder: %s", string(out))
	}
}

func TestRedactJSONForLog_EmptyInput(t *testing.T) {
	if got := RedactJSONForLog(nil); got != nil {
		t.Errorf("nil should pass through, got %q", string(got))
	}
	if got := RedactJSONForLog([]byte{}); len(got) != 0 {
		t.Errorf("empty should pass through, got %q", string(got))
	}
}

func TestRedactJSONForLog_JSONPrimitiveAtRoot(t *testing.T) {
	cases := []string{`"just-a-string"`, `42`, `true`, `null`}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			out := RedactJSONForLog([]byte(raw))
			if string(out) != raw {
				t.Errorf("primitive root mutated: want %q, got %q", raw, string(out))
			}
		})
	}
}

func TestRedactJSONForLog_DeeplyNested(t *testing.T) {
	in := []byte(`{"a":{"b":{"c":{"password":"hunter2","ok":true}}}}`)
	out := decodeRedacted(t, in)

	leaf := out["a"].(map[string]any)["b"].(map[string]any)["c"].(map[string]any)
	if leaf["password"] != redactedPlaceholder {
		t.Errorf("deep password not redacted: %v", leaf["password"])
	}
	if leaf["ok"] != true {
		t.Errorf("sibling primitive mutated: %v", leaf["ok"])
	}
}

func TestRedactJSONForLog_DoesNotMatchUnrelatedKeys(t *testing.T) {
	in := []byte(`{"name":"alice","tokenizer":"bpe","accessLevel":"admin"}`)
	out := decodeRedacted(t, in)

	if out["name"] != "alice" || out["tokenizer"] != "bpe" || out["accessLevel"] != "admin" {
		t.Errorf("non-sensitive keys mutated: %v", out)
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for _, key := range []string{"token", "Token", " ACCESSTOKEN ", "refresh_token", "API_KEY", "authorization"} {
		if !isSensitiveKey(key) {
			t.Errorf("%q should be sensitive", key)
		}
	}
	for _, key := range []string{"name", "id", "tokenizer", "passwordless"} {
		if isSensitiveKey(key) {
			t.Errorf("%q should NOT be sensitive", key)
		}
	}
}

func decodeRedacted(t *testing.T, in []byte) map[string]any {
	t.Helper()
	out := map[string]any{}
	if err := json.Unmarshal(RedactJSONForLog(in), &out); err != nil {
		t.Fatalf("redacted output not valid JSON: %v (raw: %s)", err, string(RedactJSONForLog(in)))
	}
	return out
}
