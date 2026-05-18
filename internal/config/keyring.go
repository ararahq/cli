package config

import (
	"fmt"
	"os"

	"github.com/zalando/go-keyring"
)

const ServiceName = "ararahq-cli"

func StoreAPIKey(profileName string, apiKey string) error {
	if profileName == "" {
		return fmt.Errorf("profile name cannot be empty when storing API key")
	}

	if apiKey == "" {
		return fmt.Errorf("API key cannot be empty")
	}

	keyringError := keyring.Set(ServiceName, profileName, apiKey)
	if keyringError == nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "WARNING: OS keyring unavailable (%v). Storing API key in config file instead.\n", keyringError)

	return storeAPIKeyInConfigFallback(profileName, apiKey)
}

func GetAPIKey(profileName string) (string, error) {
	if profileName == "" {
		return "", fmt.Errorf("profile name cannot be empty when retrieving API key")
	}

	apiKey, keyringError := keyring.Get(ServiceName, profileName)
	if keyringError == nil {
		return apiKey, nil
	}

	return getAPIKeyFromConfigFallback(profileName)
}

func DeleteAPIKey(profileName string) error {
	if profileName == "" {
		return fmt.Errorf("profile name cannot be empty when deleting API key")
	}

	keyringError := keyring.Delete(ServiceName, profileName)
	if keyringError == nil {
		return nil
	}

	return deleteAPIKeyFromConfigFallback(profileName)
}

func storeAPIKeyInConfigFallback(profileName string, apiKey string) error {
	configuration, loadError := Load()
	if loadError != nil {
		return fmt.Errorf("failed to load config for API key fallback storage: %w", loadError)
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		profile = Profile{
			APIURL: DefaultAPIURL,
			Mode:   DefaultMode,
		}
	}

	profile.APIKey = apiKey
	configuration.Profiles[profileName] = profile

	if saveError := Save(configuration); saveError != nil {
		return fmt.Errorf("failed to save API key to config file: %w", saveError)
	}

	return nil
}

func getAPIKeyFromConfigFallback(profileName string) (string, error) {
	configuration, loadError := Load()
	if loadError != nil {
		return "", fmt.Errorf("failed to load config for API key fallback retrieval: %w", loadError)
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		return "", fmt.Errorf("profile %q not found in config", profileName)
	}

	if profile.APIKey == "" {
		return "", fmt.Errorf("no API key found for profile %q — run 'arara auth login' to configure", profileName)
	}

	return profile.APIKey, nil
}

func deleteAPIKeyFromConfigFallback(profileName string) error {
	configuration, loadError := Load()
	if loadError != nil {
		return fmt.Errorf("failed to load config for API key fallback deletion: %w", loadError)
	}

	profile, exists := configuration.Profiles[profileName]
	if !exists {
		return nil
	}

	profile.APIKey = ""
	configuration.Profiles[profileName] = profile

	if saveError := Save(configuration); saveError != nil {
		return fmt.Errorf("failed to save config after API key removal: %w", saveError)
	}

	return nil
}
