package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	whatsappChannelPrefix = "whatsapp:"
	phoneNumberPrefix     = "+"
	messagesBasePath      = "/v1/messages"
)

// SendMessageRequest mirrors the Kotlin SendMessageRequest DTO. Every field is
// camelCase except scheduledAt, which the backend binds with
// @JsonProperty("scheduled_at") and no @JsonAlias. Sending "scheduledAt" is
// dropped silently (FAIL_ON_UNKNOWN_PROPERTIES is off), so the message goes out
// immediately and the caller never sees an error.
type SendMessageRequest struct {
	Receiver          string   `json:"receiver"`
	TemplateName      string   `json:"templateName,omitempty"`
	TemplateVariables []string `json:"variables,omitempty"`
	Body              string   `json:"body,omitempty"`
	ScheduledAt       string   `json:"scheduled_at,omitempty"`
}

type DryRunPreview struct {
	Method        string                    `json:"method"`
	Path          string                    `json:"path"`
	Payload       SendMessageRequest        `json:"payload"`
	EstimatedCost *CampaignEstimateResponse `json:"estimatedCost,omitempty"`
	EstimateError string                    `json:"estimateError,omitempty"`
}

type MessageResponse struct {
	ID           string  `json:"id"`
	Receiver     string  `json:"receiver"`
	TemplateName string  `json:"templateName,omitempty"`
	Body         string  `json:"body,omitempty"`
	Status       string  `json:"status"`
	MessageType  string  `json:"messageType,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	Sender       string  `json:"sender,omitempty"`
	Cost         float64 `json:"cost,omitempty"`
	CreatedAt    string  `json:"createdAt,omitempty"`
}

// UnmarshalJSON tolerates `"id": null` from the backend. The Kotlin DTO
// declares `id: String?`, so although today every response has the id
// populated, the contract permits null. Without this custom unmarshaller,
// `json.Unmarshal` fails on null with "cannot unmarshal null into string"
// — we'd silently turn a successful send into an error. Mapping null → ""
// lets the CLI surface a "no id returned" message to the user instead.
func (response *MessageResponse) UnmarshalJSON(data []byte) error {
	type alias MessageResponse
	aux := &struct {
		ID *string `json:"id"`
		*alias
	}{
		alias: (*alias)(response),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if aux.ID != nil {
		response.ID = *aux.ID
	}
	return nil
}

func (client *Client) SendMessage(request SendMessageRequest) (*MessageResponse, error) {
	request.Receiver = normalizeReceiver(request.Receiver)

	var response MessageResponse
	if postError := client.Post(messagesBasePath, request, &response); postError != nil {
		return nil, fmt.Errorf("failed to send message: %w", postError)
	}

	return &response, nil
}

func (client *Client) GetMessageStatus(messageID string) (*MessageResponse, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message ID cannot be empty")
	}

	path := fmt.Sprintf("%s/%s", messagesBasePath, messageID)

	var response MessageResponse
	if getError := client.Get(path, &response); getError != nil {
		return nil, fmt.Errorf("failed to get message status: %w", getError)
	}

	return &response, nil
}

func NormalizeReceiver(receiver string) string {
	return normalizeReceiver(receiver)
}

func normalizeReceiver(receiver string) string {
	normalized := receiver

	if strings.HasPrefix(normalized, whatsappChannelPrefix) {
		numberPart := strings.TrimPrefix(normalized, whatsappChannelPrefix)
		numberPart = ensurePhonePrefix(numberPart)
		return whatsappChannelPrefix + numberPart
	}

	normalized = ensurePhonePrefix(normalized)

	return whatsappChannelPrefix + normalized
}

func ensurePhonePrefix(phoneNumber string) string {
	if strings.HasPrefix(phoneNumber, phoneNumberPrefix) {
		return phoneNumber
	}

	return phoneNumberPrefix + phoneNumber
}
