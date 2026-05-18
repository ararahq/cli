package version

import (
	"strings"
	"testing"
)

func TestFull_TruncatesLongCommit(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit
	originalDate := Date
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
		Date = originalDate
	})

	Version = "1.2.3"
	Commit = "abcdef1234567890"
	Date = "2026-05-09"

	got := Full()
	if !strings.Contains(got, "v1.2.3") {
		t.Errorf("Full should include version, got %q", got)
	}
	if !strings.Contains(got, "abcdef1") {
		t.Errorf("Full should include 7-char commit, got %q", got)
	}
	if strings.Contains(got, "abcdef12") {
		t.Errorf("Full should truncate commit at 7 chars, got %q", got)
	}
	if !strings.Contains(got, "2026-05-09") {
		t.Errorf("Full should include date, got %q", got)
	}
}

func TestFull_ShortCommitNotTruncated(t *testing.T) {
	originalCommit := Commit
	t.Cleanup(func() { Commit = originalCommit })

	Commit = "abc"
	got := Full()
	if !strings.Contains(got, "abc") {
		t.Errorf("Full should include short commit verbatim, got %q", got)
	}
}
