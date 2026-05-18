package mcp

import (
	"context"
	"errors"
	"fmt"

	mcpsdk "github.com/mark3labs/mcp-go/mcp"

	"github.com/ararahq/cli/internal/api"
)

const (
	maxPageSize  = 100
	emptyMessage = ""
)

func (server *Server) handleListTemplates(_ context.Context, _ mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	templates, listErr := server.apiClient.ListTemplates()
	if listErr != nil {
		return errorResult(listErr)
	}
	return successResult(templates)
}

func (server *Server) handleGetTemplateStatus(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	templateID, idErr := request.RequireString(argTemplateID)
	if idErr != nil {
		return unexpectedArgumentError(argTemplateID, idErr)
	}

	statusResponse, fetchErr := server.apiClient.GetTemplateStatus(templateID)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(statusResponse)
}

func (server *Server) handleGetMessageStatus(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	messageID, idErr := request.RequireString(argMessageID)
	if idErr != nil {
		return unexpectedArgumentError(argMessageID, idErr)
	}

	statusResponse, fetchErr := server.apiClient.GetMessageStatus(messageID)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(statusResponse)
}

func (server *Server) handleListContacts(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	query := optionalString(request, argQuery, "")
	page := clampPage(optionalInt(request, argPage, defaultPage))
	size := clampPageSize(optionalInt(request, argSize, defaultPageSize))

	contacts, listErr := server.apiClient.ListContacts(query, page, size)
	if listErr != nil {
		return errorResult(listErr)
	}
	return successResult(contacts)
}

func (server *Server) handleGetContact(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	phone, phoneErr := request.RequireString(argPhone)
	if phoneErr != nil {
		return unexpectedArgumentError(argPhone, phoneErr)
	}

	contact, fetchErr := server.apiClient.GetContact(phone)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(contact)
}

func (server *Server) handleGetContactStats(_ context.Context, _ mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	stats, fetchErr := server.apiClient.GetContactStats()
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(stats)
}

func (server *Server) handleGetMetrics(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	mode := optionalString(request, argMode, server.apiClient.Mode())

	metrics, fetchErr := server.apiClient.GetMetrics(mode)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(metrics)
}

func (server *Server) handleGetWalletBalance(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	mode := optionalString(request, argMode, server.apiClient.Mode())

	balance, fetchErr := server.apiClient.GetWalletBalance(mode)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(balance)
}

func (server *Server) handleListNumbers(_ context.Context, _ mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	numbers, listErr := server.apiClient.ListNumbers()
	if listErr != nil {
		return errorResult(listErr)
	}
	return successResult(numbers)
}

func (server *Server) handleEstimateCampaign(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	templateName, nameErr := request.RequireString(argTemplateName)
	if nameErr != nil {
		return unexpectedArgumentError(argTemplateName, nameErr)
	}

	recipientCount, countErr := request.RequireInt(argRecipientCount)
	if countErr != nil {
		return unexpectedArgumentError(argRecipientCount, countErr)
	}
	if recipientCount <= 0 {
		return validationErrorResult("recipientCount must be greater than zero")
	}

	estimate, fetchErr := server.apiClient.EstimateCampaign(templateName, recipientCount)
	if fetchErr != nil {
		return errorResult(fetchErr)
	}
	return successResult(estimate)
}

func (server *Server) handleSendMessage(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	to, toErr := request.RequireString(argTo)
	if toErr != nil {
		return unexpectedArgumentError(argTo, toErr)
	}

	templateName := optionalString(request, argTemplateName, "")
	body := optionalString(request, argBody, "")
	variables, _ := request.RequireStringSlice(argVariables)
	scheduledAt := optionalString(request, argScheduledAt, "")
	dryRun := optionalBool(request, argDryRun, false)
	idempotencyKey := optionalString(request, argIdempotencyKey, "")

	if templateName == "" && body == "" {
		return validationErrorResult("either templateName or body is required")
	}
	if templateName != "" && body != "" {
		return validationErrorResult("templateName and body are mutually exclusive")
	}

	sendRequest := api.SendMessageRequest{
		Receiver:          api.NormalizeReceiver(to),
		TemplateName:      templateName,
		TemplateVariables: variables,
		Body:              body,
		ScheduledAt:       scheduledAt,
	}

	if dryRun {
		return server.dryRunSendMessage(sendRequest)
	}

	if idempotencyKey != "" {
		response, sendErr := server.sendMessageWithIdempotencyKey(sendRequest, idempotencyKey)
		if sendErr != nil {
			return errorResult(sendErr)
		}
		return successResult(response)
	}

	response, sendErr := server.apiClient.SendMessage(sendRequest)
	if sendErr != nil {
		return errorResult(sendErr)
	}
	return successResult(response)
}

func (server *Server) handleCreateCampaign(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	campaignName, nameErr := request.RequireString(argCampaignName)
	if nameErr != nil {
		return unexpectedArgumentError(argCampaignName, nameErr)
	}
	templateName, templateErr := request.RequireString(argTemplateName)
	if templateErr != nil {
		return unexpectedArgumentError(argTemplateName, templateErr)
	}

	contacts, contactsErr := parseCampaignContacts(request)
	if contactsErr != nil {
		return validationErrorResult(contactsErr.Error())
	}
	if len(contacts) == 0 {
		return validationErrorResult("contacts list cannot be empty")
	}

	idempotencyKey := optionalString(request, argIdempotencyKey, "")

	campaign, createErr := server.apiClient.CreateCampaign(api.CreateCampaignRequest{
		Name:         campaignName,
		TemplateName: templateName,
		Contacts:     contacts,
	}, idempotencyKey)
	if createErr != nil {
		return errorResult(createErr)
	}
	return successResult(campaign)
}

func (server *Server) handleImportContacts(_ context.Context, request mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
	contacts, parseErr := parseContactRequests(request)
	if parseErr != nil {
		return validationErrorResult(parseErr.Error())
	}
	if len(contacts) == 0 {
		return validationErrorResult("contacts list cannot be empty")
	}

	importID, importErr := server.apiClient.ImportContacts(contacts)
	if importErr != nil {
		return errorResult(importErr)
	}
	return successResult(map[string]string{"importId": importID})
}

func (server *Server) dryRunSendMessage(sendRequest api.SendMessageRequest) (*mcpsdk.CallToolResult, error) {
	preview := api.DryRunPreview{
		Method:  "POST",
		Path:    "/v1/messages",
		Payload: sendRequest,
	}

	if sendRequest.TemplateName != "" {
		estimate, estimateErr := server.apiClient.EstimateCampaign(sendRequest.TemplateName, 1)
		if estimateErr == nil {
			preview.EstimatedCost = estimate
		} else {
			preview.EstimateError = estimateErr.Error()
		}
	} else {
		preview.EstimateError = "cost estimate is only available for template messages"
	}

	return successResult(preview)
}

func (server *Server) sendMessageWithIdempotencyKey(sendRequest api.SendMessageRequest, idempotencyKey string) (*api.MessageResponse, error) {
	sendRequest.Receiver = api.NormalizeReceiver(sendRequest.Receiver)
	var response api.MessageResponse
	if postErr := server.apiClient.DoWithHeaders(
		"POST",
		"/v1/messages",
		sendRequest,
		&response,
		map[string]string{"Idempotency-Key": idempotencyKey},
	); postErr != nil {
		return nil, fmt.Errorf("failed to send message: %w", postErr)
	}
	return &response, nil
}

func parseCampaignContacts(request mcpsdk.CallToolRequest) ([]api.CampaignContact, error) {
	rawList, exists := request.GetArguments()[argContacts]
	if !exists {
		return nil, errors.New("contacts argument is required")
	}
	asSlice, ok := rawList.([]any)
	if !ok {
		return nil, fmt.Errorf("contacts must be an array, got %T", rawList)
	}

	contacts := make([]api.CampaignContact, 0, len(asSlice))
	for index, raw := range asSlice {
		entry, isObject := raw.(map[string]any)
		if !isObject {
			return nil, fmt.Errorf("contacts[%d] must be an object", index)
		}
		to, hasTo := entry[argTo].(string)
		if !hasTo || to == emptyMessage {
			return nil, fmt.Errorf("contacts[%d].to is required", index)
		}
		variables := stringSliceFromAny(entry[argVariables])
		contacts = append(contacts, api.CampaignContact{To: to, Variables: variables})
	}
	return contacts, nil
}

func parseContactRequests(request mcpsdk.CallToolRequest) ([]api.ContactRequest, error) {
	rawList, exists := request.GetArguments()[argContacts]
	if !exists {
		return nil, errors.New("contacts argument is required")
	}
	asSlice, ok := rawList.([]any)
	if !ok {
		return nil, fmt.Errorf("contacts must be an array, got %T", rawList)
	}

	contacts := make([]api.ContactRequest, 0, len(asSlice))
	for index, raw := range asSlice {
		entry, isObject := raw.(map[string]any)
		if !isObject {
			return nil, fmt.Errorf("contacts[%d] must be an object", index)
		}
		phone, hasPhone := entry["phone"].(string)
		if !hasPhone || phone == emptyMessage {
			return nil, fmt.Errorf("contacts[%d].phone is required", index)
		}
		contactRequest := api.ContactRequest{
			Phone: phone,
		}
		if name, ok := entry["name"].(string); ok {
			contactRequest.Name = name
		}
		if email, ok := entry["email"].(string); ok {
			contactRequest.Email = email
		}
		if attributes, ok := entry["attributes"].(map[string]any); ok {
			contactRequest.Attributes = attributes
		}
		contacts = append(contacts, contactRequest)
	}
	return contacts, nil
}

func stringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if asString, ok := raw.(string); ok {
				out = append(out, asString)
			}
		}
		return out
	default:
		return nil
	}
}

func optionalString(request mcpsdk.CallToolRequest, key string, fallback string) string {
	args := request.GetArguments()
	value, exists := args[key]
	if !exists {
		return fallback
	}
	stringValue, ok := value.(string)
	if !ok {
		return fallback
	}
	return stringValue
}

func optionalInt(request mcpsdk.CallToolRequest, key string, fallback int) int {
	args := request.GetArguments()
	value, exists := args[key]
	if !exists {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return fallback
	}
}

func optionalBool(request mcpsdk.CallToolRequest, key string, fallback bool) bool {
	args := request.GetArguments()
	value, exists := args[key]
	if !exists {
		return fallback
	}
	boolValue, ok := value.(bool)
	if !ok {
		return fallback
	}
	return boolValue
}

func clampPage(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampPageSize(value int) int {
	if value <= 0 {
		return defaultPageSize
	}
	if value > maxPageSize {
		return maxPageSize
	}
	return value
}
