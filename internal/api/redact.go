package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const redactedPlaceholder = "<REDACTED>"

// sensitiveJSONKeys lists JSON field names whose values must never appear in
// verbose stderr logs. Match is case-insensitive against the lowercase form.
var sensitiveJSONKeys = map[string]struct{}{
	"token":         {},
	"accesstoken":   {},
	"access_token":  {},
	"refreshtoken":  {},
	"refresh_token": {},
	"apikey":        {},
	"api_key":       {},
	"secret":        {},
	"password":      {},
	"authorization": {},
}

// RedactJSONForLog returns a copy of raw with sensitive field values replaced
// by "<REDACTED>". When raw is not valid JSON it returns a placeholder noting
// the byte length, so opaque token formats never reach stderr by accident.
//
// The redaction walks nested objects and arrays. Empty input returns an empty
// slice so verbose log lines render cleanly without extra branches at the
// call site.
func RedactJSONForLog(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}

	var parsed any
	if unmarshalErr := json.Unmarshal(raw, &parsed); unmarshalErr != nil {
		return []byte(fmt.Sprintf("<non-json body redacted (%d bytes)>", len(raw)))
	}

	redacted := redactValue(parsed)

	encoded, marshalErr := json.Marshal(redacted)
	if marshalErr != nil {
		return []byte(fmt.Sprintf("<unencodable redacted body (%d bytes)>", len(raw)))
	}
	return encoded
}

func redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveKey(key) {
				out[key] = redactedPlaceholder
				continue
			}
			out[key] = redactValue(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = redactValue(item)
		}
		return out
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	_, exists := sensitiveJSONKeys[strings.ToLower(strings.TrimSpace(key))]
	return exists
}
