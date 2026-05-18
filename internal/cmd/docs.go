package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/output"
)

const (
	docsURL          = "https://docs.ararahq.com"
	platformDarwin   = "darwin"
	platformLinux    = "linux"
	platformWindows  = "windows"
	windowsShell     = "cmd"
	windowsShellFlag = "/c"
	windowsOpenCmd   = "start"
	linuxOpenCmd     = "xdg-open"
	darwinOpenCmd    = "open"
)

// browserOpener is the function actually used to launch the browser. Tests
// override it to assert the URL we'd open without spawning a real process.
var browserOpener = openBrowserDefault

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Open the AraraHQ documentation in your browser",
	Long:  "Opens the AraraHQ developer documentation at docs.ararahq.com\nin your default web browser.",
	RunE:  runDocs,
}

func init() {
	rootCmd.AddCommand(docsCmd)
}

func runDocs(command *cobra.Command, arguments []string) error {
	if openError := browserOpener(docsURL); openError != nil {
		output.PrintError(fmt.Sprintf("Failed to open browser: %s", openError.Error()))
		output.PrintInfo(fmt.Sprintf("Open manually: %s", docsURL))
		return openError
	}

	output.PrintSuccess(fmt.Sprintf("Opening %s in your browser...", docsURL))

	return nil
}

func openBrowserDefault(url string) error {
	switch runtime.GOOS {
	case platformDarwin:
		return exec.Command(darwinOpenCmd, url).Start()
	case platformLinux:
		return exec.Command(linuxOpenCmd, url).Start()
	case platformWindows:
		return exec.Command(windowsShell, windowsShellFlag, windowsOpenCmd, url).Start()
	default:
		return fmt.Errorf("unsupported platform %q -- open %s manually", runtime.GOOS, url)
	}
}
