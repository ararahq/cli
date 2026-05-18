// Package commands discovers and parses user-defined slash commands stored
// as Markdown files with YAML frontmatter. The cascade matches settings:
// project-local commands (./.arara/commands/, walking up the directory tree)
// override user-level commands (~/.arara/commands/), which in turn lose to
// any built-in Cobra command or PATH-resolved plugin.
package commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	UserCommandsDirName    = ".arara"
	UserCommandsSubdir     = "commands"
	ProjectCommandsDirName = ".arara"
	ProjectCommandsSubdir  = "commands"

	commandFileExtension = ".md"

	SourceUser    = "user"
	SourceProject = "project"

	frontmatterDelimiter = "---"
)

// Command is a single discovered slash command. Built-in fields come from
// the file body; metadata fields come from the YAML frontmatter.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	ArgHint     string `json:"argHint,omitempty"`
	Confirm     bool   `json:"confirm,omitempty"`
	Cwd         string `json:"cwd,omitempty"`
	Body        string `json:"body"`
	Source      string `json:"source"`
	SourcePath  string `json:"sourcePath"`
}

type frontmatterFields struct {
	Description string `yaml:"description"`
	ArgHint     string `yaml:"argHint"`
	Confirm     bool   `yaml:"confirm"`
	Cwd         string `yaml:"cwd"`
}

// Loader resolves the discovery directories.
//
// Both UserDir and the project directory walk default to canonical paths if
// the corresponding fields are empty. Tests override them with t.TempDir().
type Loader struct {
	UserDir       string
	ProjectAnchor string
}

func NewLoader(workingDir string) *Loader {
	return &Loader{
		UserDir:       defaultUserDir(),
		ProjectAnchor: workingDir,
	}
}

// Discover returns every command found in user + project scopes. On name
// collision, project wins (consistent with settings cascade). Sorted by
// name for stable output.
func (loader *Loader) Discover() ([]Command, error) {
	merged := map[string]Command{}

	for _, command := range loader.loadFromDir(loader.UserDir, SourceUser) {
		merged[command.Name] = command
	}

	projectDir, projectErr := findProjectCommandsDir(loader.ProjectAnchor)
	if projectErr == nil {
		for _, command := range loader.loadFromDir(projectDir, SourceProject) {
			merged[command.Name] = command
		}
	}

	commands := make([]Command, 0, len(merged))
	for _, command := range merged {
		commands = append(commands, command)
	}
	sort.Slice(commands, func(left, right int) bool {
		return commands[left].Name < commands[right].Name
	})

	return commands, nil
}

// Lookup is a convenience for the dispatch path.
func (loader *Loader) Lookup(name string) (*Command, error) {
	commands, err := loader.Discover()
	if err != nil {
		return nil, err
	}
	for index := range commands {
		if commands[index].Name == name {
			return &commands[index], nil
		}
	}
	return nil, fmt.Errorf("slash command %q not found", name)
}

func (loader *Loader) loadFromDir(directory, source string) []Command {
	if directory == "" {
		return nil
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}

	out := make([]Command, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != commandFileExtension {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		command, parseErr := parseCommandFile(path, source)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %s (%v)\n", path, parseErr)
			continue
		}
		out = append(out, command)
	}
	return out
}

func parseCommandFile(path string, source string) (Command, error) {
	bytes, readErr := os.ReadFile(path)
	if readErr != nil {
		return Command{}, fmt.Errorf("read: %w", readErr)
	}

	frontmatter, body, splitErr := splitFrontmatter(string(bytes))
	if splitErr != nil {
		return Command{}, splitErr
	}

	fields := frontmatterFields{}
	if frontmatter != "" {
		if unmarshalErr := yaml.Unmarshal([]byte(frontmatter), &fields); unmarshalErr != nil {
			return Command{}, fmt.Errorf("invalid frontmatter: %w", unmarshalErr)
		}
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return Command{}, errors.New("command body is empty after frontmatter")
	}

	name := commandNameFromPath(path)
	if name == "" {
		return Command{}, fmt.Errorf("could not derive command name from %s", path)
	}

	description := fields.Description
	if description == "" {
		description = firstLine(body)
	}

	return Command{
		Name:        name,
		Description: description,
		ArgHint:     fields.ArgHint,
		Confirm:     fields.Confirm,
		Cwd:         fields.Cwd,
		Body:        body,
		Source:      source,
		SourcePath:  path,
	}, nil
}

// splitFrontmatter extracts the YAML frontmatter (between two `---` lines at
// the top of the file) from the body. If there is no frontmatter the whole
// content is body.
func splitFrontmatter(content string) (string, string, error) {
	trimmed := strings.TrimLeft(content, "\n")
	if !strings.HasPrefix(trimmed, frontmatterDelimiter) {
		return "", content, nil
	}

	rest := strings.TrimPrefix(trimmed, frontmatterDelimiter)
	rest = strings.TrimLeft(rest, "\n")

	endIndex := strings.Index(rest, "\n"+frontmatterDelimiter)
	if endIndex < 0 {
		return "", "", errors.New("frontmatter not closed — expected a terminating '---' line")
	}

	frontmatter := rest[:endIndex]
	body := rest[endIndex+len("\n"+frontmatterDelimiter):]
	body = strings.TrimLeft(body, "\n")
	return frontmatter, body, nil
}

func commandNameFromPath(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, commandFileExtension)
}

func firstLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return trimmed
	}
	return ""
}

func defaultUserDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", UserCommandsDirName, UserCommandsSubdir)
	}
	return filepath.Join(home, UserCommandsDirName, UserCommandsSubdir)
}

func findProjectCommandsDir(anchor string) (string, error) {
	if anchor == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return "", cwdErr
		}
		anchor = cwd
	}

	current := anchor
	for {
		candidate := filepath.Join(current, ProjectCommandsDirName, ProjectCommandsSubdir)
		if stat, statErr := os.Stat(candidate); statErr == nil && stat.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fs.ErrNotExist
		}
		current = parent
	}
}
