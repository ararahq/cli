package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNormalizeShellName(t *testing.T) {
	cases := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"bash", "bash", false},
		{"  ZSH ", "zsh", false},
		{"fish", "fish", false},
		{"PowerShell", "powershell", false},
		{"pwsh", "powershell", false},
		{"", "", true},
		{"banana", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := normalizeShellName(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Errorf("normalizeShellName(%q): want (%q,nil), got (%q,%v)", tc.input, tc.want, got, err)
			}
		})
	}
}

func TestDetectShellFromEnv(t *testing.T) {
	cases := map[string]string{
		"/bin/bash":               shellBash,
		"/usr/local/bin/zsh":      shellZsh,
		"/opt/homebrew/bin/fish":  shellFish,
		"C:\\Program Files\\pwsh": "",
		"pwsh":                    shellPowerShell,
		"/usr/bin/powershell":     shellPowerShell,
		"":                        "",
		"/something/strange":      "",
	}
	for input, want := range cases {
		t.Run(input, func(t *testing.T) {
			if got := detectShellFromEnv(input); got != want {
				t.Errorf("detectShellFromEnv(%q): want %q, got %q", input, want, got)
			}
		})
	}
}

func TestResolveCompletionShell(t *testing.T) {
	t.Setenv("SHELL", "/bin/zsh")

	got, err := resolveCompletionShell(nil, "")
	if err != nil || got != shellZsh {
		t.Errorf("auto-detect: want zsh, got (%q,%v)", got, err)
	}

	got, err = resolveCompletionShell([]string{"fish"}, "")
	if err != nil || got != shellFish {
		t.Errorf("positional arg: want fish, got (%q,%v)", got, err)
	}

	got, err = resolveCompletionShell(nil, "bash")
	if err != nil || got != shellBash {
		t.Errorf("flag value: want bash, got (%q,%v)", got, err)
	}

	t.Setenv("SHELL", "/bin/dash")
	if _, err := resolveCompletionShell(nil, ""); err == nil {
		t.Error("unsupported shell from env should error")
	}
}

func TestCompletionInstallPath(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)

	cases := map[string]string{
		shellBash:       filepath.Join(tempHome, ".bash_completion.d", "arara"),
		shellZsh:        filepath.Join(tempHome, ".zsh", "completions", "_arara"),
		shellFish:       filepath.Join(tempHome, ".config", "fish", "completions", "arara.fish"),
		shellPowerShell: filepath.Join(tempHome, ".config", "powershell", "arara.ps1"),
	}
	for shell, want := range cases {
		t.Run(shell, func(t *testing.T) {
			got, err := completionInstallPath(shell)
			if err != nil || got != want {
				t.Errorf("completionInstallPath(%q): want %q, got (%q,%v)", shell, want, got, err)
			}
		})
	}

	if _, err := completionInstallPath("powershell-foo"); err == nil {
		t.Error("expected error on unsupported shell")
	}
}

func TestWriteCompletionScript(t *testing.T) {
	for _, shell := range []string{shellBash, shellZsh, shellFish, shellPowerShell} {
		t.Run(shell, func(t *testing.T) {
			buffer := &bytes.Buffer{}
			if err := writeCompletionScript(rootCmd, shell, buffer); err != nil {
				t.Fatalf("writeCompletionScript(%s): %v", shell, err)
			}
			if buffer.Len() == 0 {
				t.Errorf("script for %s is empty", shell)
			}
		})
	}

	if err := writeCompletionScript(rootCmd, "tcsh", &bytes.Buffer{}); err == nil {
		t.Error("expected error for unsupported shell")
	}
}

func TestRunCompletionInstall_WritesFile(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("SHELL", "/bin/zsh")

	previousShellFlag := completionInstallShellFlag
	t.Cleanup(func() { completionInstallShellFlag = previousShellFlag })
	completionInstallShellFlag = ""

	if err := runCompletionInstall(&cobra.Command{}, nil); err != nil {
		t.Fatalf("runCompletionInstall: %v", err)
	}

	expectedPath := filepath.Join(tempHome, ".zsh", "completions", "_arara")
	contents, readError := os.ReadFile(expectedPath)
	if readError != nil {
		t.Fatalf("expected completion file at %s: %v", expectedPath, readError)
	}
	if !strings.Contains(string(contents), "arara") {
		t.Errorf("completion script should mention 'arara', got first 80 bytes: %q", string(contents[:min(80, len(contents))]))
	}
}

func TestPostInstallHint(t *testing.T) {
	if hint := postInstallHint(shellBash, "/x/arara"); !strings.Contains(hint, "source") {
		t.Errorf("bash hint should mention source, got %q", hint)
	}
	if hint := postInstallHint(shellZsh, "/x/_arara"); !strings.Contains(hint, "fpath") {
		t.Errorf("zsh hint should mention fpath, got %q", hint)
	}
	if postInstallHint(shellFish, "") == "" {
		t.Error("fish hint should not be empty")
	}
	if postInstallHint(shellPowerShell, "/x/arara.ps1") == "" {
		t.Error("powershell hint should not be empty")
	}
	if postInstallHint("zorg", "") != "" {
		t.Error("unknown shell should yield empty hint")
	}
}

func TestRunCompletionGenerate_BashWritesScript(t *testing.T) {
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = originalStdout })

	if err := runCompletionGenerate(nil, []string{"bash"}); err != nil {
		t.Fatalf("runCompletionGenerate: %v", err)
	}
	_ = w.Close()

	buffer := &bytes.Buffer{}
	_, _ = buffer.ReadFrom(r)
	if !strings.Contains(buffer.String(), "complete") {
		// Use Go 1.21+ builtin min to bound the slice without redefining it.
		t.Errorf("bash completion should contain 'complete', got first 200 chars: %q", buffer.String()[:min(200, buffer.Len())])
	}
}
