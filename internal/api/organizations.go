package api

import "fmt"

const organizationsPath = "/auth/me/organizations"

// Organization is a single org the authenticated user belongs to. Returned
// by GET /auth/me/organizations. Backend reads the user_organizations M2M
// table and unions in the user's primary org, so a single user can show up
// in N orgs with different roles per org. The CLI uses this to drive
// `arara org list` / `arara org use`.
type Organization struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role"`
	Mode string `json:"mode"`
}

type organizationsListResponse struct {
	Data []Organization `json:"data"`
}

// ListOrganizations fetches every organization accessible to the current
// authenticated user. Used by `arara org list` and `arara org use`.
func (client *Client) ListOrganizations() ([]Organization, error) {
	var response organizationsListResponse
	if getError := client.Get(organizationsPath, &response); getError != nil {
		return nil, fmt.Errorf("list organizations: %w", getError)
	}
	return response.Data, nil
}
