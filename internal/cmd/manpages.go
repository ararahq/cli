package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/ararahq/cli/internal/output"
)

const (
	manPagesDirMode  = 0o755
	manPagesSection  = "1"
	manPagesTitle    = "AraraHQ"
	manPagesProgram  = "arara"
	manPagesUseDate  = "2006-01-02"
	manPagesShortMsg = "Manage AraraHQ from the terminal"
)

var manpagesOutputDirFlag string

var manpagesCmd = &cobra.Command{
	Use:    "generate-manpages [output-dir]",
	Short:  "Generate man pages for the CLI",
	Hidden: true,
	Args:   cobra.MaximumNArgs(1),
	RunE:   runGenerateManpages,
}

func init() {
	manpagesCmd.Flags().StringVar(&manpagesOutputDirFlag, "output-dir", "dist/man", "directory to write man pages to")
	rootCmd.AddCommand(manpagesCmd)
}

func runGenerateManpages(_ *cobra.Command, arguments []string) error {
	outputDir := manpagesOutputDirFlag
	if len(arguments) == 1 {
		outputDir = arguments[0]
	}

	header := &doc.GenManHeader{
		Title:   manPagesTitle,
		Section: manPagesSection,
		Date:    timestampPointer(),
		Source:  manPagesProgram,
		Manual:  manPagesShortMsg,
	}

	if mkdirError := os.MkdirAll(outputDir, manPagesDirMode); mkdirError != nil {
		return fmt.Errorf("failed to create man pages output directory: %w", mkdirError)
	}

	if generateError := doc.GenManTree(rootCmd, header, outputDir); generateError != nil {
		return fmt.Errorf("failed to generate man pages: %w", generateError)
	}

	output.PrintSuccess(fmt.Sprintf("Generated man pages in %s", outputDir))
	return nil
}

func timestampPointer() *time.Time {
	now := time.Now().UTC()
	return &now
}
