package cmd

import "github.com/ararahq/cli/internal/api"

// newClientForCmd is the indirection every Cobra wrapper goes through to
// build the API client from disk-resident config + keyring. Tests swap it
// with t.Cleanup-restored shims so they can assert wrapper behavior
// without touching real keyring or config files.
var newClientForCmd = api.NewClientFromConfig
