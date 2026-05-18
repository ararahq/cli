package plugins

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	PluginPrefix              = "arara-"
	descriptionFlag           = "--plugin-description"
	defaultDescriptionTimeout = 2 * time.Second
)

type Plugin struct {
	Name        string
	BinaryPath  string
	Description string
}

type Discoverer struct {
	PathEnv      string
	ExtraDirs    []string
	Disabled     map[string]struct{}
	probeCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func NewDiscoverer(pathEnv string, extraDirs []string, disabled []string) *Discoverer {
	disabledSet := make(map[string]struct{}, len(disabled))
	for _, name := range disabled {
		disabledSet[strings.TrimSpace(name)] = struct{}{}
	}
	return &Discoverer{
		PathEnv:      pathEnv,
		ExtraDirs:    extraDirs,
		Disabled:     disabledSet,
		probeCommand: exec.CommandContext,
	}
}

func (discoverer *Discoverer) Discover() ([]Plugin, error) {
	directories := splitPathList(discoverer.PathEnv)
	directories = append(directories, discoverer.ExtraDirs...)

	seen := make(map[string]struct{})
	plugins := []Plugin{}

	for _, directory := range directories {
		if directory == "" {
			continue
		}
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasPrefix(name, PluginPrefix) {
				continue
			}

			pluginName := strings.TrimPrefix(name, PluginPrefix)
			pluginName = strings.TrimSuffix(pluginName, ".exe")
			if pluginName == "" {
				continue
			}
			if _, exists := seen[pluginName]; exists {
				continue
			}
			if _, isDisabled := discoverer.Disabled[pluginName]; isDisabled {
				continue
			}
			if !isExecutable(entry, filepath.Join(directory, name)) {
				continue
			}
			seen[pluginName] = struct{}{}

			plugins = append(plugins, Plugin{
				Name:       pluginName,
				BinaryPath: filepath.Join(directory, name),
			})
		}
	}

	sort.Slice(plugins, func(left, right int) bool {
		return plugins[left].Name < plugins[right].Name
	})

	for index := range plugins {
		plugins[index].Description = discoverer.fetchDescription(plugins[index].BinaryPath)
	}

	return plugins, nil
}

func (discoverer *Discoverer) fetchDescription(binaryPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultDescriptionTimeout)
	defer cancel()

	command := discoverer.probeCommand(ctx, binaryPath, descriptionFlag)
	output, runErr := command.Output()
	if runErr != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

type Executor struct {
	GlobalEnv map[string]string
	Stdin     *os.File
	Stdout    *os.File
	Stderr    *os.File

	execCommand func(ctx context.Context, name string, args ...string) *exec.Cmd
}

func NewExecutor() *Executor {
	return &Executor{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		execCommand: exec.CommandContext,
	}
}

func (executor *Executor) Execute(ctx context.Context, plugin Plugin, arguments []string) error {
	if plugin.BinaryPath == "" {
		return errors.New("plugin has no binary path")
	}

	command := executor.execCommand(ctx, plugin.BinaryPath, arguments...)
	command.Env = mergeEnv(os.Environ(), executor.GlobalEnv)
	command.Stdin = executor.Stdin
	command.Stdout = executor.Stdout
	command.Stderr = executor.Stderr

	runErr := command.Run()
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			return fmt.Errorf("plugin %q exited with code %d", plugin.Name, exitErr.ExitCode())
		}
		return fmt.Errorf("plugin %q failed: %w", plugin.Name, runErr)
	}
	return nil
}

func splitPathList(value string) []string {
	if value == "" {
		return nil
	}
	separator := string(os.PathListSeparator)
	parts := strings.Split(value, separator)
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func ExpandPath(path string) (string, error) {
	expanded := strings.TrimSpace(path)
	if expanded == "" {
		return "", errors.New("plugin search path is empty")
	}

	if strings.HasPrefix(expanded, "~/") || expanded == "~" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", homeErr
		}
		expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~"))
	}

	if !filepath.IsAbs(expanded) {
		absolute, absErr := filepath.Abs(expanded)
		if absErr != nil {
			return "", absErr
		}
		return absolute, nil
	}
	return expanded, nil
}

func mergeEnv(base []string, extras map[string]string) []string {
	if len(extras) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extras))
	out = append(out, base...)
	for key, value := range extras {
		out = append(out, key+"="+value)
	}
	return out
}

func isExecutable(entry fs.DirEntry, fullPath string) bool {
	info, err := entry.Info()
	if err != nil {
		return false
	}
	mode := info.Mode()
	if mode.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		lowered := strings.ToLower(fullPath)
		return strings.HasSuffix(lowered, ".exe") || strings.HasSuffix(lowered, ".bat") || strings.HasSuffix(lowered, ".cmd")
	}
	return mode&0o111 != 0
}
