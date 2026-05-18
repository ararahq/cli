package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"

	"github.com/ararahq/cli/internal/api"
)

type toolErrorEnvelope struct {
	OK         bool          `json:"ok"`
	Error      toolErrorBody `json:"error"`
	StatusCode int           `json:"statusCode,omitempty"`
}

type toolErrorBody struct {
	Code       string `json:"code,omitempty"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RetryAfter int    `json:"retryAfterMs,omitempty"`
}

type toolSuccessEnvelope struct {
	OK   bool `json:"ok"`
	Data any  `json:"data"`
}

func successResult(data any) (*mcpsdk.CallToolResult, error) {
	payload := toolSuccessEnvelope{OK: true, Data: data}
	return marshalResult(payload, false)
}

func errorResult(err error) (*mcpsdk.CallToolResult, error) {
	body := classifyError(err)
	payload := toolErrorEnvelope{OK: false, Error: body}

	var apiError *api.APIError
	if errors.As(err, &apiError) {
		payload.StatusCode = apiError.StatusCode
	}

	return marshalResult(payload, true)
}

func validationErrorResult(message string) (*mcpsdk.CallToolResult, error) {
	payload := toolErrorEnvelope{
		OK: false,
		Error: toolErrorBody{
			Code:    "ararahq-invalid-argument",
			Message: message,
		},
	}
	return marshalResult(payload, true)
}

func classifyError(err error) toolErrorBody {
	if err == nil {
		return toolErrorBody{Message: "unknown error"}
	}

	var apiError *api.APIError
	if errors.As(err, &apiError) {
		body := toolErrorBody{
			Code:    apiError.Code,
			Message: apiError.FriendlyMessage(),
		}
		if apiError.StatusCode == httpTooManyRequests || apiError.StatusCode >= 500 {
			body.Retryable = true
		}
		if apiError.RetryAfter > 0 {
			body.RetryAfter = int(apiError.RetryAfter.Milliseconds())
		}
		return body
	}

	return toolErrorBody{Message: err.Error(), Retryable: true}
}

func marshalResult(payload any, isError bool) (*mcpsdk.CallToolResult, error) {
	encoded, marshalErr := json.MarshalIndent(payload, "", "  ")
	if marshalErr != nil {
		return mcpsdk.NewToolResultErrorf("failed to encode tool result: %v", marshalErr), nil
	}

	result := mcpsdk.NewToolResultText(string(encoded))
	result.IsError = isError
	return result, nil
}

func unexpectedArgumentError(field string, cause error) (*mcpsdk.CallToolResult, error) {
	return validationErrorResult(fmt.Sprintf("argument %q is required and must be valid: %v", field, cause))
}

const httpTooManyRequests = 429
