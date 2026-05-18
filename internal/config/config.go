package config

import (
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const (
	DefaultAPIURL  = "https://api.ararahq.com/api"
	ConfigDir      = ".arara"
	ConfigFileName = "config.yaml"
	DefaultOutput  = "table"
	DefaultMode    = "test"

	defaultProfileName   = "default"
	configDirPermission  = 0700
	configFilePermission = 0600
)

type Config struct {
	CurrentProfile string             `yaml:"current_profile"`
	Output         string             `yaml:"output"`
	Profiles       map[string]Profile `yaml:"profiles"`
}

type Profile struct {
	APIKey   string `yaml:"api_key"`
	APIURL   string `yaml:"api_url"`
	Mode     string `yaml:"mode"`
	AuthType string `yaml:"auth_type,omitempty"`
}

const (
	AuthTypeAPIKey = "apikey"
	AuthTypeOAuth  = "oauth"
)

func ConfigPath() string {
	homeDirectory, homeError := os.UserHomeDir()
	if homeError != nil {
		return filepath.Join(".", ConfigDir, ConfigFileName)
	}

	return filepath.Join(homeDirectory, ConfigDir, ConfigFileName)
}

func configDirectory() string {
	homeDirectory, homeError := os.UserHomeDir()
	if homeError != nil {
		return filepath.Join(".", ConfigDir)
	}

	return filepath.Join(homeDirectory, ConfigDir)
}

func EnsureConfigDir() error {
	directory := configDirectory()

	return os.MkdirAll(directory, configDirPermission)
}

func Load() (*Config, error) {
	configFilePath := ConfigPath()

	if _, statError := os.Stat(configFilePath); os.IsNotExist(statError) {
		defaultConfig := createDefaultConfig()

		if saveError := Save(defaultConfig); saveError != nil {
			return nil, fmt.Errorf("failed to create default config file: %w", saveError)
		}

		return defaultConfig, nil
	}

	fileBytes, readError := os.ReadFile(configFilePath)
	if readError != nil {
		return nil, fmt.Errorf("failed to read config file at %s: %w", configFilePath, readError)
	}

	configuration := &Config{}
	if unmarshalError := yaml.Unmarshal(fileBytes, configuration); unmarshalError != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", unmarshalError)
	}

	if configuration.Profiles == nil {
		configuration.Profiles = make(map[string]Profile)
	}

	if configuration.Output == "" {
		configuration.Output = DefaultOutput
	}

	return configuration, nil
}

func Save(configuration *Config) error {
	if ensureError := EnsureConfigDir(); ensureError != nil {
		return fmt.Errorf("failed to create config directory: %w", ensureError)
	}

	yamlBytes, marshalError := yaml.Marshal(configuration)
	if marshalError != nil {
		return fmt.Errorf("failed to serialize config to YAML: %w", marshalError)
	}

	configFilePath := ConfigPath()

	if writeError := os.WriteFile(configFilePath, yamlBytes, configFilePermission); writeError != nil {
		return fmt.Errorf("failed to write config file at %s: %w", configFilePath, writeError)
	}

	return nil
}

func GetActiveProfile(configuration *Config) (*Profile, error) {
	if configuration.CurrentProfile == "" {
		return nil, fmt.Errorf("no active profile set — run 'arara auth login' to configure")
	}

	profile, exists := configuration.Profiles[configuration.CurrentProfile]
	if !exists {
		return nil, fmt.Errorf(
			"profile %q not found in config — available profiles: %s",
			configuration.CurrentProfile,
			availableProfileNames(configuration),
		)
	}

	if profile.APIURL == "" {
		profile.APIURL = DefaultAPIURL
	}

	if profile.Mode == "" {
		profile.Mode = DefaultMode
	}

	return &profile, nil
}

func createDefaultConfig() *Config {
	return &Config{
		CurrentProfile: defaultProfileName,
		Output:         DefaultOutput,
		Profiles: map[string]Profile{
			defaultProfileName: {
				APIURL: DefaultAPIURL,
				Mode:   DefaultMode,
			},
		},
	}
}

func availableProfileNames(configuration *Config) string {
	if len(configuration.Profiles) == 0 {
		return "(none)"
	}

	names := ""
	for profileName := range configuration.Profiles {
		if names != "" {
			names += ", "
		}
		names += profileName
	}

	return names
}
