package api

import (
	"fmt"
)

const (
	contactsBasePath    = "/v1/contacts"
	contactsBatchPath   = "/v1/contacts/batch"
	contactsStatsPath   = "/v1/contacts/stats"
	defaultContactsPage = 0
	defaultContactsSize = 20
)

type ContactRequest struct {
	Name       string         `json:"name"`
	Phone      string         `json:"phone"`
	Email      string         `json:"email,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type ContactResponse struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Phone      string         `json:"phone"`
	Email      string         `json:"email,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
	CreatedAt  string         `json:"createdAt"`
}

type ContactsListResponse struct {
	Contacts   []ContactResponse `json:"contacts"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	Size       int               `json:"size"`
	TotalPages int               `json:"totalPages"`
}

type contactsBatchResponse struct {
	ImportID string `json:"importId"`
}

func (client *Client) ListContacts(query string, page int, size int) (*ContactsListResponse, error) {
	if page < 0 {
		page = defaultContactsPage
	}

	if size <= 0 {
		size = defaultContactsSize
	}

	path := fmt.Sprintf("%s?page=%d&size=%d", contactsBasePath, page, size)
	if query != "" {
		path = fmt.Sprintf("%s&q=%s", path, query)
	}

	var response ContactsListResponse
	if getError := client.Get(path, &response); getError != nil {
		return nil, fmt.Errorf("failed to list contacts: %w", getError)
	}

	return &response, nil
}

func (client *Client) GetContact(phone string) (*ContactResponse, error) {
	if phone == "" {
		return nil, fmt.Errorf("phone number cannot be empty")
	}

	path := fmt.Sprintf("%s/%s", contactsBasePath, phone)

	var response ContactResponse
	if getError := client.Get(path, &response); getError != nil {
		return nil, fmt.Errorf("failed to get contact: %w", getError)
	}

	return &response, nil
}

func (client *Client) ImportContacts(contacts []ContactRequest) (string, error) {
	if len(contacts) == 0 {
		return "", fmt.Errorf("contacts list cannot be empty for import")
	}

	var response contactsBatchResponse
	if postError := client.Post(contactsBatchPath, contacts, &response); postError != nil {
		return "", fmt.Errorf("failed to import contacts: %w", postError)
	}

	return response.ImportID, nil
}

func (client *Client) GetContactStats() (map[string]int64, error) {
	var stats map[string]int64
	if getError := client.Get(contactsStatsPath, &stats); getError != nil {
		return nil, fmt.Errorf("failed to get contact stats: %w", getError)
	}

	return stats, nil
}
