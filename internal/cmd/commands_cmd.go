package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/commands"
	"github.com/ararahq/cli/internal/output"
)

const (
	commandsListLabelWidth = 22
)

// commandsLoaderFactory is the indirection the runCommands* wrappers go
// through to build the slash-command loader. Tests swap it for a Loader
// pointing at t.TempDir() instead of $HOME so they can drive the wrappers
// without polluting the real user dir.
var commandsLoaderFactory = func() *commands.Loader {
	return commands.NewLoader("")
}

var commandsCmd = &cobra.Command{
	Use:   "commands",
	Short: "Inspect user-defined slash commands",
	Long: "List, show, and locate user-defined slash commands discovered from\n" +
		"~/.arara/commands/ and ./.arara/commands/ (walking up the directory tree).",
}

var commandsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List discovered slash commands",
	RunE:  runCommandsList,
}

var commandsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show the body and metadata of a slash command",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommandsShow,
}

var commandsWhichCmd = &cobra.Command{
	Use:   "which <name>",
	Short: "Print the source file path of a slash command",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommandsWhich,
}

func init() {
	commandsCmd.AddCommand(commandsListCmd)
	commandsCmd.AddCommand(commandsShowCmd)
	commandsCmd.AddCommand(commandsWhichCmd)
	rootCmd.AddCommand(commandsCmd)
}

func runCommandsList(_ *cobra.Command, _ []string) error {
	loader := commandsLoaderFactory()
	discovered, err := loader.Discover()
	if err != nil {
		return err
	}

	switch GetOutputFormat() {
	case output.FormatJSON:
		return output.PrintJSON(discovered)
	case output.FormatStreamJSON:
		for _, command := range discovered {
			if writeErr := output.PrintJSONLine(command); writeErr != nil {
				return writeErr
			}
		}
		return nil
	default:
		printCommandsListTable(discovered)
		return nil
	}
}

func runCommandsShow(_ *cobra.Command, arguments []string) error {
	name := arguments[0]

	loader := commandsLoaderFactory()
	command, lookupErr := loader.Lookup(name)
	if lookupErr != nil {
		return lookupErr
	}

	if GetOutputFormat() == output.FormatJSON {
		return output.PrintJSON(command)
	}

	fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("name:"), command.Name)
	fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("source:"), command.Source)
	fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("path:"), command.SourcePath)
	if command.Description != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("description:"), command.Description)
	}
	if command.ArgHint != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("argHint:"), command.ArgHint)
	}
	if command.Confirm {
		fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("confirm:"), output.WarningStyle.Render("true (prompts before running)"))
	}
	if command.Cwd != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", output.DimStyle.Render("cwd:"), command.Cwd)
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, output.DimStyle.Render("body:"))
	fmt.Fprintln(os.Stdout, command.Body)
	return nil
}

func runCommandsWhich(_ *cobra.Command, arguments []string) error {
	name := arguments[0]

	loader := commandsLoaderFactory()
	command, lookupErr := loader.Lookup(name)
	if lookupErr != nil {
		return lookupErr
	}

	fmt.Fprintln(os.Stdout, command.SourcePath)
	return nil
}

func printCommandsListTable(discovered []commands.Command) {
	if len(discovered) == 0 {
		output.PrintInfo("No slash commands found. Create one in ~/.arara/commands/<name>.md")
		return
	}

	for _, command := range discovered {
		nameRendered := output.BoldStyle.Render(command.Name)
		sourceTag := output.DimStyle.Render("(" + command.Source + ")")
		desc := command.Description
		if desc == "" {
			desc = output.DimStyle.Render("(no description)")
		}
		fmt.Fprintf(os.Stdout, "  %-*s %s %s\n", commandsListLabelWidth, nameRendered, sourceTag, desc)
	}
}
