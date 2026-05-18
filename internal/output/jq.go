// Package output's jq.go embeds gojq so callers can apply a jq expression
// to any JSON-shaped result before rendering. Lets users write
// `arara templates list --jq '.[] | select(.status=="APPROVED") | .name'`
// without piping to an external `jq` binary.
package output

import (
	"encoding/json"
	"fmt"

	"github.com/itchyny/gojq"
)

// JQError is returned when a jq filter cannot be compiled or runs into a
// fatal error during iteration. Carries the original expression so error
// messages stay actionable.
type JQError struct {
	Expression string
	Cause      error
}

func (err *JQError) Error() string {
	return fmt.Sprintf("jq filter %q: %s", err.Expression, err.Cause.Error())
}

func (err *JQError) Unwrap() error {
	return err.Cause
}

// ApplyJQ runs the given jq expression against `input`, returning the slice
// of matched results. An empty expression is a no-op that returns the input
// wrapped in a single-element slice (so callers always render at least one
// thing).
//
// The function works on Go-native values (`map[string]any`, `[]any`,
// scalars). Callers that have raw bytes should ApplyJQRaw instead.
func ApplyJQ(input any, expression string) ([]any, error) {
	if expression == "" {
		return []any{input}, nil
	}

	query, parseErr := gojq.Parse(expression)
	if parseErr != nil {
		return nil, &JQError{Expression: expression, Cause: parseErr}
	}

	results := make([]any, 0, 1)
	iterator := query.Run(input)
	for {
		value, hasNext := iterator.Next()
		if !hasNext {
			break
		}
		if iterErr, isErr := value.(error); isErr {
			return nil, &JQError{Expression: expression, Cause: iterErr}
		}
		results = append(results, value)
	}
	return results, nil
}

// ApplyJQRaw decodes raw JSON bytes, runs the filter, and returns the
// matched values. A pass-through helper for `arara api` and similar paths
// that hold the response as bytes rather than typed structs.
func ApplyJQRaw(rawJSON []byte, expression string) ([]any, error) {
	var decoded any
	if unmarshalErr := json.Unmarshal(rawJSON, &decoded); unmarshalErr != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", unmarshalErr)
	}
	return ApplyJQ(decoded, expression)
}
