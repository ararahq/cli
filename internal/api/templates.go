package api

import (
	"fmt"
)

const (
	templatesBasePath = "/v1/templates"
)

type Template struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Category       string `json:"category"`
	Language       string `json:"language"`
	Body           string `json:"body"`
	Header         string `json:"header,omitempty"`
	Footer         string `json:"footer,omitempty"`
	ProviderStatus string `json:"providerStatus,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
}

type TemplateStatusResponse struct {
	Status          string `json:"status"`
	RejectionReason string `json:"rejectionReason,omitempty"`
	Category        string `json:"category"`
}

func (client *Client) ListTemplates() ([]Template, error) {
	var templates []Template
	if getError := client.Get(templatesBasePath, &templates); getError != nil {
		return nil, fmt.Errorf("failed to list templates: %w", getError)
	}

	return templates, nil
}

func (client *Client) GetTemplateStatus(templateID string) (*TemplateStatusResponse, error) {
	if templateID == "" {
		return nil, fmt.Errorf("template ID cannot be empty")
	}

	path := fmt.Sprintf("%s/%s/status", templatesBasePath, templateID)

	var statusResponse TemplateStatusResponse
	if getError := client.Get(path, &statusResponse); getError != nil {
		return nil, fmt.Errorf("failed to get template status: %w", getError)
	}

	return &statusResponse, nil
}

func (client *Client) DeleteTemplate(templateID string) error {
	if templateID == "" {
		return fmt.Errorf("template ID cannot be empty")
	}

	path := fmt.Sprintf("%s/%s", templatesBasePath, templateID)

	if deleteError := client.Delete(path); deleteError != nil {
		return fmt.Errorf("failed to delete template: %w", deleteError)
	}

	return nil
}
