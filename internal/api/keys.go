package api

import (
	"fmt"
)

const (
	// #nosec G101 -- HTTP path constant, not a credential.
	apiKeysBasePath = "/v1/api-keys"
)

type APIKeyInfo struct {
	ID         string `json:"id"`
	Prefix     string `json:"prefix"`
	LastFour   string `json:"lastFour"`
	Mode       string `json:"mode"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
}

type GeneratedAPIKey struct {
	PlainTextKey string `json:"plainTextKey"`
}

func (client *Client) ListAPIKeys() ([]APIKeyInfo, error) {
	var apiKeys []APIKeyInfo
	if getError := client.Get(apiKeysBasePath, &apiKeys); getError != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", getError)
	}

	return apiKeys, nil
}

func (client *Client) CreateAPIKey(mode, name string) (*GeneratedAPIKey, error) {
	if mode == "" {
		return nil, fmt.Errorf("mode cannot be empty — use 'live' or 'test'")
	}
	if name == "" {
		return nil, fmt.Errorf("name cannot be empty — describe where the key will be used")
	}

	path := fmt.Sprintf("%s?mode=%s", apiKeysBasePath, mode)
	body := map[string]string{"name": name}

	var generatedKey GeneratedAPIKey
	if postError := client.Post(path, body, &generatedKey); postError != nil {
		return nil, fmt.Errorf("failed to create API key: %w", postError)
	}

	return &generatedKey, nil
}

func (client *Client) RevokeAPIKey(keyID string) error {
	if keyID == "" {
		return fmt.Errorf("API key ID cannot be empty")
	}

	path := fmt.Sprintf("%s/%s", apiKeysBasePath, keyID)

	if deleteError := client.Delete(path); deleteError != nil {
		return fmt.Errorf("failed to revoke API key: %w", deleteError)
	}

	return nil
}
