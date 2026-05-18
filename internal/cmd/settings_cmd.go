package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/output"
	"github.com/ararahq/cli/internal/settings"
)

const settingsLabelWidth = 22

var (
	settingsListShowSourceFlag bool
	settingsSetProjectFlag     bool
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage CLI settings (theme, hooks, plugins)",
	Long: "View and modify non-credential CLI settings. Settings cascade across three layers:\n" +
		"  defaults → user (~/.arara/settings.json) → project (./.arara/settings.json walking up).\n" +
		"Use --source to see where each value came from.",
}

var settingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show all resolved settings (with source)",
	RunE:  runSettingsList,
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <path>",
	Short: "Get a single setting value (e.g. 'theme', 'hooks.preSend')",
	Args:  cobra.ExactArgs(1),
	RunE:  runSettingsGet,
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a setting value at user (default) or project scope",
	Args:  cobra.ExactArgs(2),
	RunE:  runSettingsSet,
}

func init() {
	settingsListCmd.Flags().BoolVar(&settingsListShowSourceFlag, "source", false, "show which layer (default|user|project) each value came from")
	settingsSetCmd.Flags().BoolVar(&settingsSetProjectFlag, "project", false, "write to ./.arara/settings.json instead of ~/.arara/settings.json")

	settingsCmd.AddCommand(settingsListCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)

	rootCmd.AddCommand(settingsCmd)
}

func runSettingsList(_ *cobra.Command, _ []string) error {
	resolved, loadError := settings.Load("")
	if loadError != nil {
		return loadError
	}

	if GetOutputFormat() == output.FormatJSON {
		if settingsListShowSourceFlag {
			return output.PrintJSON(resolved)
		}
		return output.PrintJSON(resolved.Settings)
	}

	printSettingsList(resolved)
	return nil
}

func runSettingsGet(_ *cobra.Command, arguments []string) error {
	dottedPath := arguments[0]

	value, source, err := settings.GetValue("", dottedPath)
	if err != nil {
		return err
	}

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(map[string]any{
			"path":   dottedPath,
			"value":  value,
			"source": source,
		})
	}

	fmt.Fprintln(os.Stdout, formatSettingsValue(value))
	return nil
}

func runSettingsSet(_ *cobra.Command, arguments []string) error {
	dottedPath := arguments[0]
	rawValue := arguments[1]

	scope := settings.ScopeUser
	if settingsSetProjectFlag {
		scope = settings.ScopeProject
	}

	parsedValue, parseError := parseSettingsValue(dottedPath, rawValue)
	if parseError != nil {
		return parseError
	}

	if writeError := settings.WriteValue(scope, "", dottedPath, parsedValue); writeError != nil {
		return writeError
	}

	output.PrintSuccess(fmt.Sprintf("Wrote %s = %s (%s scope)", dottedPath, rawValue, scope))
	return nil
}

func parseSettingsValue(dottedPath, rawValue string) (any, error) {
	switch dottedPath {
	case "theme", "output":
		return rawValue, nil
	case "hookTimeoutSeconds":
		intValue, parseError := strconv.Atoi(strings.TrimSpace(rawValue))
		if parseError != nil {
			return nil, fmt.Errorf("hookTimeoutSeconds must be an integer: %w", parseError)
		}
		return intValue, nil
	case "experimental.oauth", "mcp.allowWriteTools":
		boolValue, parseError := strconv.ParseBool(rawValue)
		if parseError != nil {
			return nil, fmt.Errorf("%s must be true|false: %w", dottedPath, parseError)
		}
		return boolValue, nil
	case "hooks.preSend", "hooks.postDeliver", "hooks.preCampaign", "hooks.postCampaign", "hooks.onError",
		"plugins.searchPaths", "plugins.disabled":
		if strings.HasPrefix(rawValue, "[") {
			var parsed []string
			if jsonError := json.Unmarshal([]byte(rawValue), &parsed); jsonError != nil {
				return nil, fmt.Errorf("could not parse %q as JSON array: %w", rawValue, jsonError)
			}
			return parsed, nil
		}
		return splitAndTrim(rawValue), nil
	default:
		return nil, fmt.Errorf("unknown setting path %q (run 'arara settings list' to see known paths)", dottedPath)
	}
}

func splitAndTrim(rawValue string) []string {
	if rawValue == "" {
		return nil
	}
	parts := strings.Split(rawValue, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func printSettingsList(resolved *settings.Resolved) {
	fmt.Fprintln(os.Stdout, output.DimStyle.Render("Sources:"))
	for _, source := range resolved.Sources {
		path := source.Path
		if path == "" {
			path = "(built-in)"
		}
		fmt.Fprintf(os.Stdout, "  %s %s\n", output.DimStyle.Render("·"), fmt.Sprintf("%s → %s", source.Name, path))
	}
	fmt.Fprintln(os.Stdout)

	for _, dottedPath := range settings.KnownPaths() {
		value, source, err := settings.GetValue("", dottedPath)
		if err != nil {
			continue
		}
		formatted := formatSettingsValue(value)
		if settingsListShowSourceFlag {
			fmt.Fprintf(os.Stdout, "  %-*s %s  %s\n", settingsLabelWidth, dottedPath+":", formatted, output.DimStyle.Render("("+source+")"))
			continue
		}
		fmt.Fprintf(os.Stdout, "  %-*s %s\n", settingsLabelWidth, dottedPath+":", formatted)
	}
}

func formatSettingsValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []string:
		if len(typed) == 0 {
			return "[]"
		}
		return "[" + strings.Join(typed, ", ") + "]"
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		encoded, marshalErr := json.Marshal(typed)
		if marshalErr != nil {
			return fmt.Sprintf("%v", typed)
		}
		return string(encoded)
	}
}
