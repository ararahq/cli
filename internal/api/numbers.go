package api

import (
	"fmt"
)

const (
	// Backend serves the simple shape we want at /organizations/me/numbers
	// (OrganizationController.listNumbers). The newer /v1/organizations/me/numbers
	// endpoint returns NumbersResponseDTO with dashboard-specific aggregates
	// (slots, billing, plan limits) we don't need for the CLI list view.
	numbersBasePath = "/organizations/me/numbers"
)

type PhoneNumberResponse struct {
	ID          string `json:"id"`
	PhoneNumber string `json:"phoneNumber"`
	Name        string `json:"name"`
}

func (client *Client) ListNumbers() ([]PhoneNumberResponse, error) {
	var numbers []PhoneNumberResponse
	if getError := client.Get(numbersBasePath, &numbers); getError != nil {
		return nil, fmt.Errorf("failed to list phone numbers: %w", getError)
	}

	return numbers, nil
}
