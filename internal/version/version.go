package version

import "fmt"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

const commitDisplayLength = 7

func Full() string {
	shortCommit := Commit
	if len(shortCommit) > commitDisplayLength {
		shortCommit = shortCommit[:commitDisplayLength]
	}

	return fmt.Sprintf("arara v%s (%s, %s)", Version, shortCommit, Date)
}
