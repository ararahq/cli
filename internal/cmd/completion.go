package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/output"
)

const (
	shellBash       = "bash"
	shellZsh        = "zsh"
	shellFish       = "fish"
	shellPowerShell = "powershell"

	completionFileMode = 0o644
	completionDirMode  = 0o755
)

var completionInstallShellFlag string

var completionInstallCmd = &cobra.Command{
	Use:   "install [shell]",
	Short: "Install shell completions for the current user",
	Long: "Generate completions for the requested shell (bash, zsh, fish, powershell)\n" +
		"and write them to the conventional path for that shell. If [shell] is omitted,\n" +
		"the shell is auto-detected from the SHELL environment variable.",
	Args: cobra.MaximumNArgs(1),
	RunE: runCompletionInstall,
}

func init() {
	completionInstallCmd.Flags().StringVar(&completionInstallShellFlag, "shell", "", "shell to target (bash, zsh, fish, powershell)")
	rootCmd.AddCommand(buildCompletionRoot())
}

func buildCompletionRoot() *cobra.Command {
	completionRoot := &cobra.Command{
		Use:                   "completion [bash|zsh|fish|powershell]",
		Short:                 "Generate or install shell completion scripts",
		Long:                  "Generate completion scripts for bash, zsh, fish, or powershell.\nUse `arara completion install` to write them to the conventional path.",
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{shellBash, shellZsh, shellFish, shellPowerShell},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE:                  runCompletionGenerate,
	}
	completionRoot.AddCommand(completionInstallCmd)
	return completionRoot
}

func runCompletionGenerate(command *cobra.Command, arguments []string) error {
	shell := arguments[0]
	return writeCompletionScript(rootCmd, shell, os.Stdout)
}

func runCompletionInstall(command *cobra.Command, arguments []string) error {
	shell, resolveError := resolveCompletionShell(arguments, completionInstallShellFlag)
	if resolveError != nil {
		return resolveError
	}

	targetPath, pathError := completionInstallPath(shell)
	if pathError != nil {
		return pathError
	}

	if mkdirError := os.MkdirAll(filepath.Dir(targetPath), completionDirMode); mkdirError != nil {
		return fmt.Errorf("failed to create completion directory: %w", mkdirError)
	}

	scriptBuffer := &bytes.Buffer{}
	if writeError := writeCompletionScript(rootCmd, shell, scriptBuffer); writeError != nil {
		return writeError
	}

	if writeError := os.WriteFile(targetPath, scriptBuffer.Bytes(), completionFileMode); writeError != nil {
		return fmt.Errorf("failed to write completion file at %s: %w", targetPath, writeError)
	}

	output.PrintSuccess(fmt.Sprintf("Wrote %s completion to %s", shell, targetPath))

	hint := postInstallHint(shell, targetPath)
	if hint != "" {
		output.PrintInfo(hint)
	}
	return nil
}

func writeCompletionScript(rootCommand *cobra.Command, shell string, writer interface {
	Write([]byte) (int, error)
},
) error {
	switch shell {
	case shellBash:
		return rootCommand.GenBashCompletionV2(writer, true)
	case shellZsh:
		return rootCommand.GenZshCompletion(writer)
	case shellFish:
		return rootCommand.GenFishCompletion(writer, true)
	case shellPowerShell:
		return rootCommand.GenPowerShellCompletionWithDesc(writer)
	default:
		return fmt.Errorf("unsupported shell: %q (want one of bash, zsh, fish, powershell)", shell)
	}
}

func resolveCompletionShell(arguments []string, flagValue string) (string, error) {
	if flagValue != "" {
		return normalizeShellName(flagValue)
	}
	if len(arguments) == 1 {
		return normalizeShellName(arguments[0])
	}
	detected := detectShellFromEnv(os.Getenv("SHELL"))
	if detected == "" {
		return "", fmt.Errorf("could not detect shell from $SHELL — pass it explicitly: 'arara completion install bash|zsh|fish|powershell'")
	}
	return detected, nil
}

func normalizeShellName(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case shellBash:
		return shellBash, nil
	case shellZsh:
		return shellZsh, nil
	case shellFish:
		return shellFish, nil
	case shellPowerShell, "pwsh":
		return shellPowerShell, nil
	default:
		return "", fmt.Errorf("unsupported shell: %q", raw)
	}
}

func detectShellFromEnv(shellEnv string) string {
	base := filepath.Base(shellEnv)
	switch base {
	case shellBash:
		return shellBash
	case shellZsh:
		return shellZsh
	case shellFish:
		return shellFish
	case "pwsh", shellPowerShell:
		return shellPowerShell
	default:
		return ""
	}
}

func completionInstallPath(shell string) (string, error) {
	homeDir, homeError := os.UserHomeDir()
	if homeError != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", homeError)
	}

	switch shell {
	case shellBash:
		return filepath.Join(homeDir, ".bash_completion.d", "arara"), nil
	case shellZsh:
		return filepath.Join(homeDir, ".zsh", "completions", "_arara"), nil
	case shellFish:
		return filepath.Join(homeDir, ".config", "fish", "completions", "arara.fish"), nil
	case shellPowerShell:
		return filepath.Join(homeDir, ".config", "powershell", "arara.ps1"), nil
	default:
		return "", fmt.Errorf("unsupported shell: %q", shell)
	}
}

func postInstallHint(shell string, targetPath string) string {
	switch shell {
	case shellBash:
		return fmt.Sprintf("Add this to ~/.bashrc if not already present: source %q", targetPath)
	case shellZsh:
		return fmt.Sprintf("Make sure ~/.zsh/completions is on $fpath. Add this to ~/.zshrc:\n  fpath=(%s $fpath)\n  autoload -Uz compinit && compinit", filepath.Dir(targetPath))
	case shellFish:
		return "Restart your fish shell or run: exec fish"
	case shellPowerShell:
		return fmt.Sprintf("Add this to your PowerShell profile: . %q", targetPath)
	default:
		return ""
	}
}
