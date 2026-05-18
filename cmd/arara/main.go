package main

import (
	"os"

	"github.com/ararahq/cli/internal/cmd"
	"github.com/ararahq/cli/internal/version"
)

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

func main() {
	version.Version = buildVersion
	version.Commit = buildCommit
	version.Date = buildDate

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
