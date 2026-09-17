package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
)

const CurrentVersion = 1

var FileNames = []string{".migrail.yaml", ".migrail.yml"}

type Config struct {
	Version  int                    `yaml:"version"`
	DB       DB                     `yaml:"db"`
	Projects []Project              `yaml:"projects"`
	Git      Git                    `yaml:"git"`
	Rules    map[string]RuleSetting `yaml:"rules"`
	Policy   Policy                 `yaml:"policy"`
	Ignore   []Ignore               `yaml:"ignore"`
	Output   Output                 `yaml:"output"`

	Path string `yaml:"-"`
}

type DB struct {
	Dialect string `yaml:"dialect"`
	Version string `yaml:"version"`
}

type Project struct {
	Name       string `yaml:"name"`
	Path       string `yaml:"path"`
	Framework  string `yaml:"framework"`
	Migrations string `yaml:"migrations"`
}

type Git struct {
	Base  string `yaml:"base"`
	Check string `yaml:"check"`
}

type RuleSetting struct {
	Severity string `yaml:"severity"`
}

type Policy struct {
	FailOn              string `yaml:"fail_on"`
	RequireIgnoreReason *bool  `yaml:"require_ignore_reason"`
}

type Ignore struct {
	Path   string `yaml:"path"`
	Rule   string `yaml:"rule"`
	Reason string `yaml:"reason"`
}

type Output struct {
	Format     string `yaml:"format"`
	Theme      string `yaml:"theme"`
	Color      string `yaml:"color"`
	Compact    bool   `yaml:"compact"`
	Hyperlinks string `yaml:"hyperlinks"`
}

func (r *RuleSetting) UnmarshalYAML(data []byte) error {
	var severity string
	if err := yaml.Unmarshal(data, &severity); err == nil {
		r.Severity = severity

		return nil
	}

	type plain RuleSetting

	var setting plain
	if err := yaml.UnmarshalWithOptions(data, &setting, yaml.DisallowUnknownField()); err != nil {
		return err
	}

	*r = RuleSetting(setting)

	return nil
}

func (c Config) RequireIgnoreReason() bool {
	return c.Policy.RequireIgnoreReason == nil || *c.Policy.RequireIgnoreReason
}

type Error struct {
	Path       string
	Line       int
	Column     int
	Message    string
	Suggestion string
	Source     string
}

func (e *Error) Error() string {
	message := fmt.Sprintf("invalid config %s:%d:%d: %s", e.Path, e.Line, e.Column, e.Message)
	if e.Suggestion != "" {
		message += fmt.Sprintf(". Did you mean %q?", e.Suggestion)
	}

	return message
}

func Find(root string) (string, bool) {
	for _, name := range FileNames {
		candidate := filepath.Join(root, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}

	return "", false
}

func Load(path string) (Config, error) {
	data, err := readSource(path)
	if err != nil {
		return Config{}, err
	}

	return Parse(path, data)
}

func Parse(path string, data []byte) (Config, error) {
	var cfg Config

	if len(bytes.TrimSpace(data)) == 0 {
		return Config{Version: CurrentVersion, Path: path}, nil
	}

	if err := yaml.UnmarshalWithOptions(data, &cfg, yaml.DisallowUnknownField()); err != nil {
		return Config{}, decodeError(path, data, err)
	}

	cfg.Path = path

	if err := validate(cfg, path, data); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

var unknownField = regexp.MustCompile(`unknown field "([^"]+)"`)

func decodeError(path string, data []byte, err error) error {
	result := &Error{Path: path, Line: 1, Column: 1, Message: err.Error(), Source: string(data)}

	var yamlErr yaml.Error
	if !errors.As(err, &yamlErr) {
		return result
	}

	result.Message = yamlErr.GetMessage()

	if token := yamlErr.GetToken(); token != nil {
		result.Line = token.Position.Line
		result.Column = token.Position.Column
	}

	if match := unknownField.FindStringSubmatch(result.Message); match != nil {
		result.Suggestion = closest(match[1], knownKeys())
	}

	return result
}

func errorAt(path string, data []byte, yamlPath, message string, options []string) *Error {
	result := &Error{Path: path, Line: 1, Column: 1, Message: message, Source: string(data)}

	if node := findNode(data, yamlPath); node != nil {
		result.Line = node.GetToken().Position.Line
		result.Column = node.GetToken().Position.Column
	}

	if len(options) > 0 {
		result.Message = fmt.Sprintf("%s: use %s", message, strings.Join(options, ", "))
	}

	return result
}

func findNode(data []byte, yamlPath string) ast.Node {
	file, err := parser.ParseBytes(data, 0)
	if err != nil {
		return nil
	}

	query, err := yaml.PathString(yamlPath)
	if err != nil {
		return nil
	}

	node, err := query.FilterFile(file)
	if err != nil {
		return nil
	}

	return node
}

func knownKeys() []string {
	return []string{
		"version", "db", "projects", "git", "rules", "policy", "ignore", "output",
		"dialect", "name", "path", "framework", "migrations", "base", "check", "severity",
		"fail_on", "require_ignore_reason", "rule", "reason", "format", "theme", "color", "compact", "hyperlinks",
	}
}

func closest(name string, candidates []string) string {
	best, bestDistance := "", max(2, len(name)/3)+1

	for _, candidate := range candidates {
		if distance := levenshtein(strings.ToLower(name), strings.ToLower(candidate)); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}

	return best
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}

		previous = current
	}

	return previous[len(b)]
}

func contains(options []string, value string) bool {
	return value == "" || slices.Contains(options, value)
}

func readSource(path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	return data, nil
}
