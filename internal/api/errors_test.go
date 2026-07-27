package api

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestParseErrorResponse_Envelope(t *testing.T) {
	body := []byte(`{"error":{"code":"ararahq-63016","message":"window closed"}}`)
	got := ParseErrorResponse(http.StatusBadRequest, body)

	if got.StatusCode != http.StatusBadRequest {
		t.Errorf("status: want 400, got %d", got.StatusCode)
	}
	if got.Code != "ararahq-63016" {
		t.Errorf("code: want ararahq-63016, got %q", got.Code)
	}
	if got.Message != "window closed" {
		t.Errorf("message mismatch: %q", got.Message)
	}
	if got.RawBody != string(body) {
		t.Errorf("raw body should be preserved")
	}
}

func TestParseErrorResponse_FlatShape(t *testing.T) {
	body := []byte(`{"code":"ararahq-rate-limit","message":"slow down"}`)
	got := ParseErrorResponse(http.StatusTooManyRequests, body)

	if got.Code != "ararahq-rate-limit" {
		t.Errorf("flat shape code mismatch: %q", got.Code)
	}
	if got.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status: want 429, got %d", got.StatusCode)
	}
}

func TestParseErrorResponse_Garbage(t *testing.T) {
	body := []byte(`<html>500</html>`)
	got := ParseErrorResponse(http.StatusInternalServerError, body)

	if got.StatusCode != http.StatusInternalServerError {
		t.Errorf("status: want 500, got %d", got.StatusCode)
	}
	if got.Code != "" {
		t.Errorf("garbage body must yield empty code, got %q", got.Code)
	}
	if !strings.Contains(got.Message, "<html>") {
		t.Errorf("garbage body should fall back to raw, got %q", got.Message)
	}
}

func TestAPIError_Error(t *testing.T) {
	withCode := &APIError{StatusCode: 400, Code: "x", Message: "y"}
	if !strings.Contains(withCode.Error(), "[400] x: y") {
		t.Errorf("expected formatted error, got %q", withCode.Error())
	}

	noCode := &APIError{StatusCode: 500, Message: "boom"}
	if !strings.Contains(noCode.Error(), "[500] boom") {
		t.Errorf("expected message-only fallback, got %q", noCode.Error())
	}

	rawOnly := &APIError{StatusCode: 503, RawBody: "raw"}
	if !strings.Contains(rawOnly.Error(), "[503] raw") {
		t.Errorf("expected raw fallback, got %q", rawOnly.Error())
	}
}

func TestAPIError_FriendlyMessage(t *testing.T) {
	known := &APIError{Code: errorCodeWindowClosed, Message: "ignored"}
	if !strings.Contains(known.FriendlyMessage(), "Janela de 24h") {
		t.Errorf("expected mapped friendly message, got %q", known.FriendlyMessage())
	}

	unknownWithMessage := &APIError{Code: "unknown", Message: "raw upstream"}
	if unknownWithMessage.FriendlyMessage() != "raw upstream" {
		t.Errorf("expected raw message fallback, got %q", unknownWithMessage.FriendlyMessage())
	}

	bare := &APIError{StatusCode: 500, RawBody: "nope"}
	if !strings.Contains(bare.FriendlyMessage(), "[500]") {
		t.Errorf("expected error-format fallback, got %q", bare.FriendlyMessage())
	}
}

func TestParseErrorResponse_ReadsDiagnosticCode(t *testing.T) {
	body := []byte(`{"error":{"code":"UNPROCESSABLE_ENTITY","message":"Janela de 24h fechada.",` +
		`"details":{"diagnostic":{"code":"ararahq-63016","whatHappened":"Janela fechada.",` +
		`"howToAct":"Envie um template."}}}}`)
	got := ParseErrorResponse(http.StatusUnprocessableEntity, body)

	if got.Code != "UNPROCESSABLE_ENTITY" {
		t.Errorf("envelope code should be preserved, got %q", got.Code)
	}
	if got.DiagnosticCode != errorCodeWindowClosed {
		t.Errorf("diagnostic code: want %q, got %q", errorCodeWindowClosed, got.DiagnosticCode)
	}
	if got.BusinessCode() != errorCodeWindowClosed {
		t.Errorf("business code should prefer the diagnostic, got %q", got.BusinessCode())
	}
	if !strings.Contains(got.FriendlyMessage(), "Janela de 24h fechada. Envie um template primeiro.") {
		t.Errorf("friendly message should be reachable, got %q", got.FriendlyMessage())
	}
}

func TestParseErrorResponse_UnknownDiagnosticFallsBackToHint(t *testing.T) {
	body := []byte(`{"error":{"code":"UNPROCESSABLE_ENTITY","message":"[131037]",` +
		`"details":{"diagnostic":{"code":"ararahq-display-name",` +
		`"whatHappened":"Nome de exibicao nao aprovado.","howToAct":"Peca a aprovacao."}}}}`)
	got := ParseErrorResponse(http.StatusUnprocessableEntity, body)

	if got.BusinessCode() != "ararahq-display-name" {
		t.Errorf("business code: got %q", got.BusinessCode())
	}
	want := "Nome de exibicao nao aprovado. Peca a aprovacao."
	if got.FriendlyMessage() != want {
		t.Errorf("friendly message: want %q, got %q", want, got.FriendlyMessage())
	}
}

func TestParseErrorResponse_DetailsWithoutDiagnostic(t *testing.T) {
	body := []byte(`{"error":{"code":"PLAN_FEATURE_LOCKED","message":"Faca upgrade.",` +
		`"details":{"feature":"campaigns","currentPlan":"FREE","upgradeTo":"VOO"}}}`)
	got := ParseErrorResponse(http.StatusForbidden, body)

	if got.BusinessCode() != "PLAN_FEATURE_LOCKED" {
		t.Errorf("business code should fall back to the envelope code, got %q", got.BusinessCode())
	}
	if got.DiagnosticCode != "" {
		t.Errorf("no diagnostic expected, got %q", got.DiagnosticCode)
	}
	if got.FriendlyMessage() != "Faca upgrade." {
		t.Errorf("friendly message: got %q", got.FriendlyMessage())
	}
}

func TestParseErrorResponse_NonObjectDetailsKeepsCode(t *testing.T) {
	body := []byte(`{"error":{"code":"VALIDATION_ERROR","message":"invalido","details":["campo"]}}`)
	got := ParseErrorResponse(http.StatusBadRequest, body)

	if got.Code != "VALIDATION_ERROR" {
		t.Errorf("unexpected details shape must not lose the code, got %q", got.Code)
	}
	if got.DiagnosticCode != "" {
		t.Errorf("no diagnostic expected, got %q", got.DiagnosticCode)
	}
}

func TestIsAuthError(t *testing.T) {
	if !IsAuthError(&APIError{StatusCode: http.StatusUnauthorized}) {
		t.Error("401 should be auth error")
	}
	if IsAuthError(&APIError{StatusCode: http.StatusForbidden}) {
		t.Error("403 should not be auth error")
	}
	if IsAuthError(errors.New("boom")) {
		t.Error("non-API error should not be auth error")
	}
	if IsAuthError(nil) {
		t.Error("nil should not be auth error")
	}
}

func TestIsNotFoundError(t *testing.T) {
	if !IsNotFoundError(&APIError{StatusCode: http.StatusNotFound}) {
		t.Error("404 should be not-found")
	}
	if IsNotFoundError(&APIError{StatusCode: http.StatusOK}) {
		t.Error("200 should not be not-found")
	}
	if IsNotFoundError(errors.New("boom")) {
		t.Error("non-API error should not be not-found")
	}
}
