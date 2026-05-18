package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	UserSettingsDirName     = ".arara"
	UserSettingsFileName    = "settings.json"
	ProjectSettingsDirName  = ".arara"
	ProjectSettingsFileName = "settings.json"

	SourceDefault = "default"
	SourceUser    = "user"
	SourceProject = "project"

	defaultHookTimeoutSeconds = 30
	settingsFileMode          = 0o600
	settingsDirMode           = 0o700
)

type Settings struct {
	Theme              string         `json:"theme,omitempty"`
	Output             string         `json:"output,omitempty"`
	Hooks              Hooks          `json:"hooks,omitempty"`
	Plugins            PluginSettings `json:"plugins,omitempty"`
	MCP                MCPSettings    `json:"mcp,omitempty"`
	HookTimeoutSeconds int            `json:"hookTimeoutSeconds,omitempty"`
	Experimental       Experimental   `json:"experimental,omitempty"`
}

type MCPSettings struct {
	AllowWriteTools bool `json:"allowWriteTools,omitempty"`
}

type Hooks struct {
	PreSend      []string `json:"preSend,omitempty"`
	PostDeliver  []string `json:"postDeliver,omitempty"`
	PreCampaign  []string `json:"preCampaign,omitempty"`
	PostCampaign []string `json:"postCampaign,omitempty"`
	OnError      []string `json:"onError,omitempty"`
}

type PluginSettings struct {
	SearchPaths []string `json:"searchPaths,omitempty"`
	Disabled    []string `json:"disabled,omitempty"`
}

type Experimental struct {
	OAuth bool `json:"oauth,omitempty"`
}

type ResolvedField struct {
	Path   string
	Value  any
	Source string
}

type Resolved struct {
	Settings Settings
	Fields   []ResolvedField
	Sources  []Source
}

type Source struct {
	Name string
	Path string
}

func Defaults() Settings {
	return Settings{
		Theme:              "auto",
		Output:             "text",
		HookTimeoutSeconds: defaultHookTimeoutSeconds,
	}
}

func UserSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", UserSettingsDirName, UserSettingsFileName)
	}
	return filepath.Join(home, UserSettingsDirName, UserSettingsFileName)
}

func ProjectSettingsPath(workingDir string) string {
	resolved, err := findProjectSettings(workingDir)
	if err != nil {
		return ""
	}
	return resolved
}

func findProjectSettings(workingDir string) (string, error) {
	if workingDir == "" {
		var cwdErr error
		workingDir, cwdErr = os.Getwd()
		if cwdErr != nil {
			return "", cwdErr
		}
	}

	current := workingDir
	for {
		candidate := filepath.Join(current, ProjectSettingsDirName, ProjectSettingsFileName)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", os.ErrNotExist
		}
		current = parent
	}
}

func Load(workingDir string) (*Resolved, error) {
	resolved := &Resolved{
		Settings: Defaults(),
	}

	resolved.Sources = append(resolved.Sources, Source{Name: SourceDefault, Path: ""})

	if userSettings, sourcePath, err := readSettingsFile(UserSettingsPath()); err == nil && userSettings != nil {
		mergeSettings(&resolved.Settings, userSettings, &resolved.Fields, SourceUser)
		resolved.Sources = append(resolved.Sources, Source{Name: SourceUser, Path: sourcePath})
	}

	if projectPath, projectErr := findProjectSettings(workingDir); projectErr == nil {
		if projectSettings, sourcePath, err := readSettingsFile(projectPath); err == nil && projectSettings != nil {
			mergeSettings(&resolved.Settings, projectSettings, &resolved.Fields, SourceProject)
			resolved.Sources = append(resolved.Sources, Source{Name: SourceProject, Path: sourcePath})
		}
	}

	return resolved, nil
}

func readSettingsFile(path string) (*Settings, string, error) {
	if path == "" {
		return nil, "", os.ErrNotExist
	}
	bytes, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, path, readErr
	}
	if len(bytes) == 0 {
		return &Settings{}, path, nil
	}

	var parsed Settings
	if unmarshalErr := json.Unmarshal(bytes, &parsed); unmarshalErr != nil {
		return nil, path, fmt.Errorf("invalid JSON in %s: %w", path, unmarshalErr)
	}
	return &parsed, path, nil
}

func mergeSettings(destination, layer *Settings, fields *[]ResolvedField, sourceName string) {
	if layer.Theme != "" {
		destination.Theme = layer.Theme
		recordField(fields, "theme", layer.Theme, sourceName)
	}
	if layer.Output != "" {
		destination.Output = layer.Output
		recordField(fields, "output", layer.Output, sourceName)
	}
	if layer.HookTimeoutSeconds > 0 {
		destination.HookTimeoutSeconds = layer.HookTimeoutSeconds
		recordField(fields, "hookTimeoutSeconds", layer.HookTimeoutSeconds, sourceName)
	}

	mergeHooks(&destination.Hooks, &layer.Hooks, fields, sourceName)
	mergePlugins(&destination.Plugins, &layer.Plugins, fields, sourceName)

	if layer.Experimental.OAuth {
		destination.Experimental.OAuth = true
		recordField(fields, "experimental.oauth", true, sourceName)
	}

	if layer.MCP.AllowWriteTools {
		destination.MCP.AllowWriteTools = true
		recordField(fields, "mcp.allowWriteTools", true, sourceName)
	}
}

func mergeHooks(destination, layer *Hooks, fields *[]ResolvedField, sourceName string) {
	if len(layer.PreSend) > 0 {
		destination.PreSend = appendUnique(destination.PreSend, layer.PreSend)
		recordField(fields, "hooks.preSend", layer.PreSend, sourceName)
	}
	if len(layer.PostDeliver) > 0 {
		destination.PostDeliver = appendUnique(destination.PostDeliver, layer.PostDeliver)
		recordField(fields, "hooks.postDeliver", layer.PostDeliver, sourceName)
	}
	if len(layer.PreCampaign) > 0 {
		destination.PreCampaign = appendUnique(destination.PreCampaign, layer.PreCampaign)
		recordField(fields, "hooks.preCampaign", layer.PreCampaign, sourceName)
	}
	if len(layer.PostCampaign) > 0 {
		destination.PostCampaign = appendUnique(destination.PostCampaign, layer.PostCampaign)
		recordField(fields, "hooks.postCampaign", layer.PostCampaign, sourceName)
	}
	if len(layer.OnError) > 0 {
		destination.OnError = appendUnique(destination.OnError, layer.OnError)
		recordField(fields, "hooks.onError", layer.OnError, sourceName)
	}
}

func mergePlugins(destination, layer *PluginSettings, fields *[]ResolvedField, sourceName string) {
	if len(layer.SearchPaths) > 0 {
		destination.SearchPaths = appendUnique(destination.SearchPaths, layer.SearchPaths)
		recordField(fields, "plugins.searchPaths", layer.SearchPaths, sourceName)
	}
	if len(layer.Disabled) > 0 {
		destination.Disabled = appendUnique(destination.Disabled, layer.Disabled)
		recordField(fields, "plugins.disabled", layer.Disabled, sourceName)
	}
}

func appendUnique(existing, additions []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additions))
	for _, item := range existing {
		seen[item] = struct{}{}
	}
	combined := make([]string, 0, len(existing)+len(additions))
	combined = append(combined, existing...)
	for _, item := range additions {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		combined = append(combined, item)
	}
	return combined
}

func recordField(fields *[]ResolvedField, path string, value any, source string) {
	*fields = append(*fields, ResolvedField{Path: path, Value: value, Source: source})
}

type WriteScope string

const (
	ScopeUser    WriteScope = "user"
	ScopeProject WriteScope = "project"
)

func WriteValue(scope WriteScope, workingDir, dottedPath string, value any) error {
	targetPath, pathError := resolveWritePath(scope, workingDir)
	if pathError != nil {
		return pathError
	}

	current, _, readError := readSettingsFile(targetPath)
	if readError != nil && !errors.Is(readError, os.ErrNotExist) {
		return readError
	}
	if current == nil {
		current = &Settings{}
	}

	if applyError := applyDottedPath(current, dottedPath, value); applyError != nil {
		return applyError
	}

	return writeSettingsFile(targetPath, current)
}

func GetValue(workingDir, dottedPath string) (any, string, error) {
	resolved, err := Load(workingDir)
	if err != nil {
		return nil, "", err
	}

	value, found := lookupDottedPath(&resolved.Settings, dottedPath)
	if !found {
		return nil, "", fmt.Errorf("setting %q not set", dottedPath)
	}

	source := SourceDefault
	for _, field := range resolved.Fields {
		if field.Path == dottedPath {
			source = field.Source
		}
	}

	return value, source, nil
}

func resolveWritePath(scope WriteScope, workingDir string) (string, error) {
	switch scope {
	case ScopeUser:
		return UserSettingsPath(), nil
	case ScopeProject:
		if workingDir == "" {
			cwd, cwdErr := os.Getwd()
			if cwdErr != nil {
				return "", cwdErr
			}
			workingDir = cwd
		}
		return filepath.Join(workingDir, ProjectSettingsDirName, ProjectSettingsFileName), nil
	default:
		return "", fmt.Errorf("unknown write scope: %q", scope)
	}
}

func writeSettingsFile(path string, settings *Settings) error {
	if mkdirErr := os.MkdirAll(filepath.Dir(path), settingsDirMode); mkdirErr != nil {
		return fmt.Errorf("failed to create settings directory: %w", mkdirErr)
	}

	encoded, marshalErr := json.MarshalIndent(settings, "", "  ")
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal settings: %w", marshalErr)
	}
	encoded = append(encoded, '\n')

	if writeErr := os.WriteFile(path, encoded, settingsFileMode); writeErr != nil {
		return fmt.Errorf("failed to write settings file at %s: %w", path, writeErr)
	}
	return nil
}

func applyDottedPath(settings *Settings, dottedPath string, value any) error {
	switch dottedPath {
	case "theme":
		stringValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("theme must be a string")
		}
		settings.Theme = stringValue
	case "output":
		stringValue, ok := value.(string)
		if !ok {
			return fmt.Errorf("output must be a string")
		}
		settings.Output = stringValue
	case "hookTimeoutSeconds":
		intValue, ok := toInt(value)
		if !ok {
			return fmt.Errorf("hookTimeoutSeconds must be an integer")
		}
		settings.HookTimeoutSeconds = intValue
	case "hooks.preSend":
		settings.Hooks.PreSend = toStringSlice(value)
	case "hooks.postDeliver":
		settings.Hooks.PostDeliver = toStringSlice(value)
	case "hooks.preCampaign":
		settings.Hooks.PreCampaign = toStringSlice(value)
	case "hooks.postCampaign":
		settings.Hooks.PostCampaign = toStringSlice(value)
	case "hooks.onError":
		settings.Hooks.OnError = toStringSlice(value)
	case "plugins.searchPaths":
		settings.Plugins.SearchPaths = toStringSlice(value)
	case "plugins.disabled":
		settings.Plugins.Disabled = toStringSlice(value)
	case "experimental.oauth":
		boolValue, ok := value.(bool)
		if !ok {
			return fmt.Errorf("experimental.oauth must be a boolean")
		}
		settings.Experimental.OAuth = boolValue
	case "mcp.allowWriteTools":
		boolValue, ok := value.(bool)
		if !ok {
			return fmt.Errorf("mcp.allowWriteTools must be a boolean")
		}
		settings.MCP.AllowWriteTools = boolValue
	default:
		return fmt.Errorf("unknown setting path: %q", dottedPath)
	}
	return nil
}

func lookupDottedPath(settings *Settings, dottedPath string) (any, bool) {
	switch dottedPath {
	case "theme":
		return settings.Theme, settings.Theme != ""
	case "output":
		return settings.Output, settings.Output != ""
	case "hookTimeoutSeconds":
		return settings.HookTimeoutSeconds, settings.HookTimeoutSeconds > 0
	case "hooks.preSend":
		return settings.Hooks.PreSend, len(settings.Hooks.PreSend) > 0
	case "hooks.postDeliver":
		return settings.Hooks.PostDeliver, len(settings.Hooks.PostDeliver) > 0
	case "hooks.preCampaign":
		return settings.Hooks.PreCampaign, len(settings.Hooks.PreCampaign) > 0
	case "hooks.postCampaign":
		return settings.Hooks.PostCampaign, len(settings.Hooks.PostCampaign) > 0
	case "hooks.onError":
		return settings.Hooks.OnError, len(settings.Hooks.OnError) > 0
	case "plugins.searchPaths":
		return settings.Plugins.SearchPaths, len(settings.Plugins.SearchPaths) > 0
	case "plugins.disabled":
		return settings.Plugins.Disabled, len(settings.Plugins.Disabled) > 0
	case "experimental.oauth":
		return settings.Experimental.OAuth, true
	case "mcp.allowWriteTools":
		return settings.MCP.AllowWriteTools, true
	default:
		return nil, false
	}
}

func KnownPaths() []string {
	return []string{
		"theme",
		"output",
		"hookTimeoutSeconds",
		"hooks.preSend",
		"hooks.postDeliver",
		"hooks.preCampaign",
		"hooks.postCampaign",
		"hooks.onError",
		"plugins.searchPaths",
		"plugins.disabled",
		"mcp.allowWriteTools",
		"experimental.oauth",
	}
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case string:
		if typed == "" {
			return nil
		}
		parts := strings.Split(typed, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(typed))
		for _, raw := range typed {
			if str, ok := raw.(string); ok {
				out = append(out, str)
			}
		}
		return out
	default:
		return nil
	}
}

func toInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		var parsed int
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil {
			return parsed, true
		}
		return 0, false
	default:
		return 0, false
	}
}
