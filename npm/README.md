# @ararahq/cli

Thin npm wrapper that downloads the matching native Go binary of the [Arara CLI](https://github.com/ararahq/cli) on install.

This package exists so users on Node-heavy stacks (CI runners, monorepos using `npx`) can do:

```bash
npm install -g @ararahq/cli
arara --help

# or one-off:
npx @ararahq/cli send --to +5511999999999 --template hello_world
```

For everyone else, prefer the native distribution:

- `brew install ararahq/tap/arara` (macOS / Linux)
- `scoop install ararahq/arara` (Windows)
- Pre-built binaries on [GitHub Releases](https://github.com/ararahq/cli/releases)
- `docker run ararahq/arara:latest`

The wrapper requires Node 16+. On install it downloads the binary from the GitHub release matching this package's version, so the npm version and the Go binary version are always 1:1.
