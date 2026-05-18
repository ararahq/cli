package cmd

import (
	"errors"
	"testing"
)

func TestRunDocs_InvokesBrowserOpenerWithDocsURL(t *testing.T) {
	originalOpener := browserOpener
	t.Cleanup(func() { browserOpener = originalOpener })

	calledWith := ""
	browserOpener = func(url string) error {
		calledWith = url
		return nil
	}

	if err := runDocs(nil, nil); err != nil {
		t.Fatalf("runDocs: %v", err)
	}
	if calledWith != docsURL {
		t.Errorf("opener called with %q, want %q", calledWith, docsURL)
	}
}

func TestRunDocs_PropagatesOpenerError(t *testing.T) {
	originalOpener := browserOpener
	t.Cleanup(func() { browserOpener = originalOpener })

	want := errors.New("no display")
	browserOpener = func(_ string) error { return want }

	err := runDocs(nil, nil)
	if !errors.Is(err, want) {
		t.Errorf("runDocs should propagate opener error, got: %v", err)
	}
}
