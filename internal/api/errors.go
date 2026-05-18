package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

const (
	errorCodeWindowClosed      = "ararahq-63016"
	errorCodeInvalidNumber     = "ararahq-63024"
	errorCodeInsufficientFunds = "ararahq-insufficient-funds"
	errorCodeRateLimit         = "ararahq-rate-limit"
	errorCodeTemplateMismatch  = "ararahq-template-mismatch"
	errorCodeOptOut            = "ararahq-opt-out"
)

var friendlyMessages = map[string]string{
	errorCodeWindowClosed:      "Janela de 24h fechada. Envie um template primeiro.",
	errorCodeInvalidNumber:     "Numero invalido. Use formato: +5511999999999",
	errorCodeInsufficientFunds: "Saldo insuficiente. Recarregue em ararahq.com/dashboard",
	errorCodeRateLimit:         "Rate limit atingido. Aguarde ou use campaigns para envio em massa.",
	errorCodeTemplateMismatch:  "Variaveis nao batem com o template.",
	errorCodeOptOut:            "Destinatario bloqueou mensagens.",
}

type APIError struct {
	StatusCode int
	Code       string `json:"code"`
	Message    string `json:"message"`
	RawBody    string
	RetryAfter time.Duration
}

func (apiError *APIError) Error() string {
	if apiError.Code != "" {
		return fmt.Sprintf("[%d] %s: %s", apiError.StatusCode, apiError.Code, apiError.Message)
	}

	if apiError.Message != "" {
		return fmt.Sprintf("[%d] %s", apiError.StatusCode, apiError.Message)
	}

	return fmt.Sprintf("[%d] %s", apiError.StatusCode, apiError.RawBody)
}

func (apiError *APIError) FriendlyMessage() string {
	friendlyMessage, exists := friendlyMessages[apiError.Code]
	if exists {
		return friendlyMessage
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
}

func ParseErrorResponse(statusCode int, body []byte) *APIError {
	rawBody := string(body)

	var envelope apiErrorEnvelope
	if unmarshalError := json.Unmarshal(body, &envelope); unmarshalError == nil && envelope.Error.Code != "" {
		return &APIError{
			StatusCode: statusCode,
			Code:       envelope.Error.Code,
			Message:    envelope.Error.Message,
			RawBody:    rawBody,
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
