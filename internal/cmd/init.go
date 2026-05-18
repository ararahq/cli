package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/output"
)

const (
	initProjectDirName     = ".arara"
	initSettingsFileName   = "settings.json"
	initCommandsSubdir     = "commands"
	initHooksSubdir        = "hooks"
	initExampleCommandFile = "deploy.md"
	initExampleHookFile    = "log-send.sh"
	initGitignoreFileName  = ".gitignore"

	initFilePerms = 0o644
	initExecPerms = 0o755
	initDirPerms  = 0o755
)

var (
	initForceFlag bool
	initDirFlag   string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Scaffold .arara/ in the current project",
	Long: "Create a .arara/ directory in the current project with sensible boilerplate:\n" +
		"  .arara/settings.json   — empty settings file (project scope overrides user)\n" +
		"  .arara/commands/       — example slash command (deploy.md)\n" +
		"  .arara/hooks/          — example pre-send hook script\n" +
		"  .gitignore             — appends .arara/credentials* if a .gitignore exists\n\n" +
		"Existing files are preserved unless --force is set.",
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initForceFlag, "force", false, "overwrite existing files")
	initCmd.Flags().StringVar(&initDirFlag, "dir", ".", "directory to scaffold into (defaults to cwd)")

	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	targetRoot, absError := filepath.Abs(initDirFlag)
	if absError != nil {
		output.PrintError(fmt.Sprintf("Could not resolve directory: %s", absError.Error()))
		return absError
	}

	report, scaffoldError := scaffoldInitProject(targetRoot, initForceFlag)
	if scaffoldError != nil {
		output.PrintError(scaffoldError.Error())
		return scaffoldError
	}

	printInitReport(os.Stdout, targetRoot, report)
	return nil
}

// initReportEntry tracks one scaffolded path and whether it was newly
// created or skipped because it already existed (and --force wasn't set).
type initReportEntry struct {
	Path    string
	Created bool
	Reason  string
}

// scaffoldInitProject is the testable core of `arara init`. Pure I/O on the
// filesystem under `root`, no global state, no stdout — returns a report
// the caller renders.
func scaffoldInitProject(root string, force bool) ([]initReportEntry, error) {
	projectDir := filepath.Join(root, initProjectDirName)
	commandsDir := filepath.Join(projectDir, initCommandsSubdir)
	hooksDir := filepath.Join(projectDir, initHooksSubdir)

	for _, dir := range []string{projectDir, commandsDir, hooksDir} {
		if mkErr := os.MkdirAll(dir, initDirPerms); mkErr != nil {
			return nil, fmt.Errorf("create %s: %w", dir, mkErr)
		}
	}

	files := []struct {
		path    string
		content []byte
		perms   os.FileMode
	}{
		{filepath.Join(projectDir, initSettingsFileName), []byte(initSettingsTemplate), initFilePerms},
		{filepath.Join(commandsDir, initExampleCommandFile), []byte(initExampleCommandTemplate), initFilePerms},
		{filepath.Join(hooksDir, initExampleHookFile), []byte(initExampleHookTemplate), initExecPerms},
	}

	report := make([]initReportEntry, 0, len(files)+1)
	for _, file := range files {
		entry, writeErr := writeIfAbsent(file.path, file.content, file.perms, force)
		if writeErr != nil {
			return nil, writeErr
		}
		report = append(report, entry)
	}

	gitignoreEntry, gitignoreErr := updateGitignore(root)
	if gitignoreErr != nil {
		return nil, gitignoreErr
	}
	if gitignoreEntry != nil {
		report = append(report, *gitignoreEntry)
	}

	return report, nil
}

func writeIfAbsent(path string, content []byte, perms os.FileMode, force bool) (initReportEntry, error) {
	if _, statErr := os.Stat(path); statErr == nil {
		if !force {
			return initReportEntry{Path: path, Created: false, Reason: "already exists"}, nil
		}
	}

	if writeErr := os.WriteFile(path, content, perms); writeErr != nil {
		return initReportEntry{}, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return initReportEntry{Path: path, Created: true}, nil
}

// updateGitignore appends a credentials-ignore entry to the project's
// .gitignore when one exists. Does not create a new .gitignore — projects
// without git or with no existing .gitignore probably manage ignores
// elsewhere.
func updateGitignore(root string) (*initReportEntry, error) {
	path := filepath.Join(root, initGitignoreFileName)

	existing, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		return nil, nil
	}
	if readErr != nil {
		return nil, fmt.Errorf("read .gitignore: %w", readErr)
	}

	const marker = ".arara/credentials"
	if strings.Contains(string(existing), marker) {
		return &initReportEntry{Path: path, Created: false, Reason: "already mentions .arara"}, nil
	}

	updated := strings.TrimRight(string(existing), "\n") + "\n\n# arara-cli\n.arara/credentials*\n"
	// #nosec G703 -- `path` is filepath.Join(root, initGitignoreFileName)
	// where root is filepath.Abs() of the user-provided --dir flag. A
	// project scaffolder writing to a user-chosen directory is its job;
	// the user is the actor with intent. Nothing third-party-influenced
	// reaches this path.
	if writeErr := os.WriteFile(path, []byte(updated), initFilePerms); writeErr != nil {
		return nil, fmt.Errorf("update .gitignore: %w", writeErr)
	}

	return &initReportEntry{Path: path, Created: true, Reason: "appended .arara/credentials*"}, nil
}

func printInitReport(writer io.Writer, root string, entries []initReportEntry) {
	output.PrintSuccess(fmt.Sprintf("Initialized .arara/ in %s", root))
	fmt.Fprintln(writer)

	for _, entry := range entries {
		relative, relErr := filepath.Rel(root, entry.Path)
		if relErr != nil {
			relative = entry.Path
		}
		if entry.Created {
			fmt.Fprintf(writer, "  %s %s\n", output.SuccessStyle.Render("+"), relative)
		} else {
			fmt.Fprintf(writer, "  %s %s %s\n", output.DimStyle.Render("·"), relative, output.DimStyle.Render("("+entry.Reason+")"))
		}
	}

	fmt.Fprintln(writer)
	output.PrintInfo("Next steps:")
	fmt.Fprintln(writer, "  1. arara login        — authenticate")
	fmt.Fprintln(writer, "  2. arara deploy       — try the example slash command (it just echoes)")
	fmt.Fprintln(writer, "  3. arara settings list --source")
}

const initSettingsTemplate = `{
  "$schema": "https://docs.ararahq.com/cli/settings.schema.json",
  "theme": "auto",
  "output": "text",
  "hooks": {
    "preSend": ["./.arara/hooks/log-send.sh"]
  }
}
`

const initExampleCommandTemplate = `---
description: Example slash command — replace with your real workflow
argHint: "<env>"
---
echo "would deploy to env=$1 (this is the example command from arara init)"
`

const initExampleHookTemplate = `#!/usr/bin/env bash
# arara pre-send hook — receives the request payload as JSON on stdin
# and lifecycle metadata via env vars (ARARA_EVENT, ARARA_PROFILE, ARARA_MODE).
# Exit 0 to proceed, non-zero to abort the send.
set -euo pipefail
mkdir -p ~/.arara
echo "[$(date -u +%FT%TZ)] $ARARA_EVENT profile=$ARARA_PROFILE" >> ~/.arara/audit.log
cat >> ~/.arara/audit.log
echo >> ~/.arara/audit.log
exit 0
`
