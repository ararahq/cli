package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGenerateManpages_WritesAllCommands(t *testing.T) {
	outputDir := t.TempDir()

	previous := manpagesOutputDirFlag
	t.Cleanup(func() { manpagesOutputDirFlag = previous })
	manpagesOutputDirFlag = outputDir

	if err := runGenerateManpages(manpagesCmd, nil); err != nil {
		t.Fatalf("runGenerateManpages: %v", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	requiredPages := []string{
		"arara.1",
		"arara-doctor.1",
		"arara-send.1",
		"arara-listen.1",
		"arara-logs.1",
		"arara-completion.1",
	}
	gotNames := map[string]struct{}{}
	for _, entry := range entries {
		gotNames[entry.Name()] = struct{}{}
	}
	for _, want := range requiredPages {
		if _, ok := gotNames[want]; !ok {
			t.Errorf("missing man page: %s", want)
		}
	}

	doctorBytes, readError := os.ReadFile(filepath.Join(outputDir, "arara-doctor.1"))
	if readError != nil {
		t.Fatalf("read doctor man page: %v", readError)
	}
	if !strings.Contains(string(doctorBytes), "doctor") {
		t.Errorf("doctor man page should mention 'doctor', got first 200 bytes: %q", string(doctorBytes[:min(200, len(doctorBytes))]))
	}
}

func TestRunGenerateManpages_PositionalOverridesFlag(t *testing.T) {
	flagDir := t.TempDir()
	argDir := t.TempDir()

	previous := manpagesOutputDirFlag
	t.Cleanup(func() { manpagesOutputDirFlag = previous })
	manpagesOutputDirFlag = flagDir

	if err := runGenerateManpages(manpagesCmd, []string{argDir}); err != nil {
		t.Fatalf("runGenerateManpages: %v", err)
	}

	if entries, _ := os.ReadDir(flagDir); len(entries) != 0 {
		t.Errorf("expected positional arg to override flag, but flag dir has %d entries", len(entries))
	}
	entries, err := os.ReadDir(argDir)
	if err != nil || len(entries) == 0 {
		t.Errorf("expected man pages in positional arg dir, got %d entries (err=%v)", len(entries), err)
	}
}
