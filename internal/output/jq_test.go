package output

import (
	"errors"
	"strings"
	"testing"
)

func TestApplyJQ_EmptyExpressionPassesThrough(t *testing.T) {
	input := map[string]any{"name": "alice"}
	results, err := ApplyJQ(input, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if got, ok := results[0].(map[string]any); !ok || got["name"] != "alice" {
		t.Errorf("input not preserved, got %v", results[0])
	}
}

func TestApplyJQ_ExtractField(t *testing.T) {
	input := map[string]any{"name": "alice", "age": 30}
	results, err := ApplyJQ(input, ".name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0] != "alice" {
		t.Errorf("want alice, got %v", results[0])
	}
}

func TestApplyJQ_ArrayIteration(t *testing.T) {
	input := []any{
		map[string]any{"name": "alice"},
		map[string]any{"name": "bob"},
		map[string]any{"name": "carol"},
	}
	results, err := ApplyJQ(input, ".[] | .name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	expected := []string{"alice", "bob", "carol"}
	for i, want := range expected {
		if results[i] != want {
			t.Errorf("results[%d]: want %q, got %v", i, want, results[i])
		}
	}
}

func TestApplyJQ_FilterWithSelect(t *testing.T) {
	input := []any{
		map[string]any{"name": "a", "status": "APPROVED"},
		map[string]any{"name": "b", "status": "PENDING"},
		map[string]any{"name": "c", "status": "APPROVED"},
	}
	results, err := ApplyJQ(input, `.[] | select(.status=="APPROVED") | .name`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d (%v)", len(results), results)
	}
	if results[0] != "a" || results[1] != "c" {
		t.Errorf("expected [a, c], got %v", results)
	}
}

func TestApplyJQ_InvalidExpressionReturnsJQError(t *testing.T) {
	_, err := ApplyJQ(map[string]any{}, "...invalid syntax...")
	if err == nil {
		t.Fatal("expected error")
	}
	var jqErr *JQError
	if !errors.As(err, &jqErr) {
		t.Errorf("expected *JQError, got %T", err)
	}
	if !strings.Contains(err.Error(), "...invalid") {
		t.Errorf("error should reference expression, got: %v", err)
	}
}

func TestApplyJQRaw_DecodesAndFilters(t *testing.T) {
	raw := []byte(`[{"name":"a"},{"name":"b"}]`)
	results, err := ApplyJQRaw(raw, ".[].name")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("want 2, got %d", len(results))
	}
}

func TestApplyJQRaw_RejectsInvalidJSON(t *testing.T) {
	_, err := ApplyJQRaw([]byte("not json"), ".")
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("error should mention JSON, got: %v", err)
	}
}

func TestApplyJQ_EmptyResultIsValid(t *testing.T) {
	input := []any{map[string]any{"status": "PENDING"}}
	results, err := ApplyJQ(input, `.[] | select(.status=="APPROVED")`)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("want 0 results when filter matches nothing, got %d", len(results))
	}
}

func TestApplyJQ_ScalarRoot(t *testing.T) {
	results, err := ApplyJQ(42, ". + 1")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
}

func TestJQError_Unwrap(t *testing.T) {
	cause := errors.New("inner")
	jqErr := &JQError{Expression: ".x", Cause: cause}
	if !errors.Is(jqErr, cause) {
		t.Error("JQError should unwrap to its cause")
	}
}
