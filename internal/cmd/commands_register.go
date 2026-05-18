package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ararahq/cli/internal/commands"
	"github.com/ararahq/cli/internal/output"
)

const (
	commandsCommandGroupID = "user-commands"
	untrustedExitCode      = 2
	trustEnvVar            = "ARARA_TRUST"
)

func registerDiscoveredCommands(rootCommand *cobra.Command) {
	loader := commands.NewLoader("")
	discovered, err := loader.Discover()
	if err != nil || len(discovered) == 0 {
		return
	}

	registerCommandsAsSubcommands(rootCommand, discovered)
}

func registerCommandsAsSubcommands(rootCommand *cobra.Command, discovered []commands.Command) {
	ensureUserCommandGroup(rootCommand)

	for _, command := range discovered {
		if commandAlreadyRegistered(rootCommand, command.Name) {
			output.PrintWarning(fmt.Sprintf("ignoring slash command %q (%s) — name conflicts with a built-in or plugin", command.Name, command.SourcePath))
			continue
		}

		commandCopy := command

		shortDescription := commandCopy.Description
		if shortDescription == "" {
			shortDescription = "user-defined slash command"
		}

		useString := commandCopy.Name
		if commandCopy.ArgHint != "" {
			useString = commandCopy.Name + " " + commandCopy.ArgHint
		}

		userCommand := &cobra.Command{
			Use:                useString,
			Short:              shortDescription,
			GroupID:            commandsCommandGroupID,
			DisableFlagParsing: true,
			Annotations:        map[string]string{"source": commandCopy.Source, "path": commandCopy.SourcePath},
			RunE: func(_ *cobra.Command, arguments []string) error {
				if commandCopy.Confirm && !confirmExecution(commandCopy) {
					output.PrintWarning(fmt.Sprintf("slash command %q aborted", commandCopy.Name))
					return nil
				}

				exitCode, runErr := commands.Execute(context.Background(), commandCopy, commands.ExecOptions{
					Args:   arguments,
					Stdin:  os.Stdin,
					Stdout: os.Stdout,
					Stderr: os.Stderr,
					Trust:  trustFromEnv(),
				})
				if runErr != nil {
					if errors.Is(runErr, commands.ErrUntrusted) {
						output.PrintError(runErr.Error())
						os.Exit(untrustedExitCode)
					}
					if exitCode > 0 {
						os.Exit(exitCode)
					}
					return runErr
				}
				return nil
			},
		}

		rootCommand.AddCommand(userCommand)
	}
}

func ensureUserCommandGroup(rootCommand *cobra.Command) {
	for _, group := range rootCommand.Groups() {
		if group.ID == commandsCommandGroupID {
			return
		}
	}
	rootCommand.AddGroup(&cobra.Group{
		ID:    commandsCommandGroupID,
		Title: "User Commands:",
	})
}

func confirmExecution(command commands.Command) bool {
	fmt.Fprintf(os.Stderr, "Run %q? [y/N] ", command.Name)
	response := make([]byte, 1)
	if _, readErr := os.Stdin.Read(response); readErr != nil {
		return false
	}
	return response[0] == 'y' || response[0] == 'Y'
}

// trustFromEnv reads ARARA_TRUST=1 to skip the interactive trust prompt for
// project-scope slash commands. Used by CI and unattended scripts. We avoid
// a Cobra flag here because slash commands run with DisableFlagParsing, so
// any --trust would collide with the slash command's own flags.
func trustFromEnv() bool {
	return os.Getenv(trustEnvVar) == "1"
}
