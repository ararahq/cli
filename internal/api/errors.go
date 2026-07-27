package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	errorCodeWindowClosed      = "ararahq-63016"
	errorCodeReengagement      = "ararahq-reengagement"
	errorCodeInvalidNumber     = "ararahq-invalid-number"
	errorCodeTemplateError     = "ararahq-63024"
	errorCodeTemplateNotFound  = "ararahq-template-not-found"
	errorCodeInsufficientFunds = "ararahq-insufficient-funds"
	errorCodeRateLimit         = "ararahq-rate-limit"
	errorCodeTemplateMismatch  = "ararahq-template-mismatch"
	errorCodeOptOut            = "ararahq-opt-out"
)

var friendlyMessages = map[string]string{
	errorCodeWindowClosed:      "Janela de 24h fechada. Envie um template primeiro.",
	errorCodeReengagement:      "Janela de 24h fechada. Envie um template primeiro.",
	errorCodeInvalidNumber:     "Numero invalido. Use formato: +5511999999999",
	errorCodeTemplateError:     "Template rejeitado ou pausado pela Meta. Confira o status no dashboard.",
	errorCodeTemplateNotFound:  "Template nao existe ou nao esta aprovado nesta conta.",
	errorCodeInsufficientFunds: "Saldo insuficiente. Recarregue em ararahq.com/dashboard",
	errorCodeRateLimit:         "Rate limit atingido. Aguarde ou use campaigns para envio em massa.",
	errorCodeTemplateMismatch:  "Variaveis nao batem com o template.",
	errorCodeOptOut:            "Destinatario bloqueou mensagens.",
}

type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
	// DiagnosticCode carries the real business code ("ararahq-63016") that the
	// backend buries in error.details.diagnostic.code. GlobalExceptionHandler
	// flattens every BusinessException to code "UNPROCESSABLE_ENTITY", so Code
	// alone never identifies what went wrong.
	DiagnosticCode string
	// DiagnosticHint is the backend-authored explanation (whatHappened +
	// howToAct) used when the code has no entry in friendlyMessages. The
	// backend emits a diagnostic for every provider code, including ones the
	// CLI does not know about.
	DiagnosticHint string
	RawBody        string
	RetryAfter     time.Duration
}

// BusinessCode returns the code that identifies the failure for the user,
// preferring the diagnostic over the generic envelope code.
func (apiError *APIError) BusinessCode() string {
	if apiError.DiagnosticCode != "" {
		return apiError.DiagnosticCode
	}

	return apiError.Code
}

func (apiError *APIError) Error() string {
	businessCode := apiError.BusinessCode()
	if businessCode != "" {
		return fmt.Sprintf("[%d] %s: %s", apiError.StatusCode, businessCode, apiError.Message)
	}

	if apiError.Message != "" {
		return fmt.Sprintf("[%d] %s", apiError.StatusCode, apiError.Message)
	}

	return fmt.Sprintf("[%d] %s", apiError.StatusCode, apiError.RawBody)
}

func (apiError *APIError) FriendlyMessage() string {
	friendlyMessage, exists := friendlyMessages[apiError.BusinessCode()]
	if exists {
		return friendlyMessage
	}

	if apiError.DiagnosticHint != "" {
		return apiError.DiagnosticHint
	}

	if apiError.Message != "" {
		return apiError.Message
	}

	return apiError.Error()
}

func IsAuthError(err error) bool {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return false
	}

	return apiError.StatusCode == http.StatusUnauthorized
}

func IsNotFoundError(err error) bool {
	var apiError *APIError
	if !errors.As(err, &apiError) {
		return false
	}

	return apiError.StatusCode == http.StatusNotFound
}

type apiErrorEnvelope struct {
	Error apiErrorPayload `json:"error"`
}

type apiErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Details is free-form per exception type: some handlers put a diagnostic
	// object in it, others put plan metadata or a list. Decoding it lazily
	// keeps an unexpected shape from failing the whole envelope and losing the
	// error code.
	Details json.RawMessage `json:"details"`
}

type apiErrorDetails struct {
	Diagnostic apiErrorDiagnostic `json:"diagnostic"`
}

type apiErrorDiagnostic struct {
	Code         string `json:"code"`
	WhatHappened string `json:"whatHappened"`
	HowToAct     string `json:"howToAct"`
}

func (diagnostic apiErrorDiagnostic) hint() string {
	parts := make([]string, 0, 2)
	for _, part := range []string{diagnostic.WhatHappened, diagnostic.HowToAct} {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}

	return strings.Join(parts, " ")
}

func extractDiagnostic(rawDetails json.RawMessage) apiErrorDiagnostic {
	if len(rawDetails) == 0 {
		return apiErrorDiagnostic{}
	}

	var details apiErrorDetails
	if unmarshalError := json.Unmarshal(rawDetails, &details); unmarshalError != nil {
		return apiErrorDiagnostic{}
	}

	return details.Diagnostic
}

func ParseErrorResponse(statusCode int, body []byte) *APIError {
	rawBody := string(body)

	var envelope apiErrorEnvelope
	if unmarshalError := json.Unmarshal(body, &envelope); unmarshalError == nil && envelope.Error.Code != "" {
		diagnostic := extractDiagnostic(envelope.Error.Details)
		return &APIError{
			StatusCode:     statusCode,
			Code:           envelope.Error.Code,
			Message:        envelope.Error.Message,
			DiagnosticCode: strings.TrimSpace(diagnostic.Code),
			DiagnosticHint: diagnostic.hint(),
			RawBody:        rawBody,
		}
	}

	var flatError APIError
	if unmarshalError := json.Unmarshal(body, &flatError); unmarshalError == nil && flatError.Code != "" {
		flatError.StatusCode = statusCode
		flatError.RawBody = rawBody
		return &flatError
	}

	return &APIError{
		StatusCode: statusCode,
		Message:    rawBody,
		RawBody:    rawBody,
	}
}
