package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var catalog = Catalog{
	RuleIDs:    []string{"MR101", "MR403", "MR603"},
	RuleSlugs:  []string{"create-index-non-concurrent", "missing-lock-timeout"},
	Frameworks: []string{"goose", "sql"},
}

func TestParseValidConfig(t *testing.T) {
	t.Parallel()

	source := `# yaml-language-server: $schema=schema.json
version: 1
db:
  dialect: postgres
  version: "16"
projects:
  - name: api
    path: services/api
    framework: goose
    migrations: db/migrations
git:
  base: origin/main
  check: all
rules:
  MR403:
    severity: error
  create-index-non-concurrent: warning
  MR603: off
policy:
  fail_on: warning
  require_ignore_reason: false
ignore:
  - path: "db/migrations/2019*"
  - rule: MR101
    path: "legacy/**"
    reason: legacy schema
output:
  format: json
  theme: mono
  compact: true
  hyperlinks: never
`

	cfg, err := Parse(".migrail.yaml", []byte(source))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if err := cfg.ValidateCatalog(catalog); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}

	checks := map[string]bool{
		"db version":        cfg.DB.Version == "16",
		"project":           len(cfg.Projects) == 1 && cfg.Projects[0].Migrations == "db/migrations",
		"rule object":       cfg.Rules["MR403"].Severity == "error",
		"rule shorthand":    cfg.Rules["create-index-non-concurrent"].Severity == "warning",
		"rule off":          cfg.Rules["MR603"].Severity == "off",
		"ignores":           len(cfg.Ignore) == 2 && cfg.Ignore[1].Reason == "legacy schema",
		"reason not needed": !cfg.RequireIgnoreReason(),
		"output":            cfg.Output.Format == "json" && cfg.Output.Compact,
	}

	for name, ok := range checks {
		if !ok {
			t.Errorf("%s not parsed correctly: %+v", name, cfg)
		}
	}
}

func TestParseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		source         string
		wantLine       int
		wantColumn     int
		wantMessage    string
		wantSuggestion string
	}{
		{
			name:           "unknown top-level key",
			source:         "version: 1\nrulez:\n  MR101: off\n",
			wantLine:       2,
			wantColumn:     1,
			wantMessage:    `unknown field "rulez"`,
			wantSuggestion: "rules",
		},
		{
			name:           "unknown nested key",
			source:         "version: 1\ndb:\n  dialct: postgres\n",
			wantLine:       3,
			wantColumn:     3,
			wantMessage:    `unknown field "dialct"`,
			wantSuggestion: "dialect",
		},
		{
			name:        "invalid enum",
			source:      "version: 1\npolicy:\n  fail_on: fatal\n",
			wantLine:    3,
			wantColumn:  12,
			wantMessage: `invalid policy.fail_on "fatal": use error, warning, notice, never`,
		},
		{
			name:        "wrong version",
			source:      "version: 2\n",
			wantLine:    1,
			wantColumn:  10,
			wantMessage: "version must be 1",
		},
		{
			name:        "unsupported database version",
			source:      "version: 1\ndb:\n  version: \"9.6\"\n",
			wantLine:    3,
			wantColumn:  12,
			wantMessage: `invalid db.version "9.6": migrail supports PostgreSQL 12 to 18`,
		},
		{
			name:       "wrong type",
			source:     "version: 1\noutput:\n  compact: maybe\n",
			wantLine:   3,
			wantColumn: 12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse(".migrail.yaml", []byte(tt.source))

			var configErr *Error
			if !errors.As(err, &configErr) {
				t.Fatalf("Parse() error = %v, want *Error", err)
			}

			if configErr.Line != tt.wantLine || configErr.Column != tt.wantColumn {
				t.Errorf("position = %d:%d, want %d:%d (%s)", configErr.Line, configErr.Column, tt.wantLine, tt.wantColumn, configErr.Message)
			}

			if tt.wantMessage != "" && configErr.Message != tt.wantMessage {
				t.Errorf("message = %q, want %q", configErr.Message, tt.wantMessage)
			}

			if configErr.Suggestion != tt.wantSuggestion {
				t.Errorf("suggestion = %q, want %q", configErr.Suggestion, tt.wantSuggestion)
			}
		})
	}
}

func TestValidateCatalog(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".migrail.yaml")
	source := "version: 1\nrules:\n  MR10: off\n"

	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err = cfg.ValidateCatalog(catalog)

	var configErr *Error
	if !errors.As(err, &configErr) || configErr.Line != 3 || configErr.Suggestion != "MR101" {
		t.Fatalf("ValidateCatalog() error = %+v", err)
	}

	if !strings.Contains(err.Error(), `Did you mean "MR101"?`) {
		t.Errorf("Error() = %q", err.Error())
	}

	bad := Config{Path: path, Projects: []Project{{Path: "api", Framework: "rails"}}}
	if err := bad.ValidateCatalog(catalog); err == nil || !strings.Contains(err.Error(), `unknown framework "rails"`) {
		t.Errorf("ValidateCatalog() with an unknown framework = %v", err)
	}
}

func TestParseEmptyAndFind(t *testing.T) {
	t.Parallel()

	cfg, err := Parse(".migrail.yaml", []byte("  \n"))
	if err != nil || cfg.Version != CurrentVersion || !cfg.RequireIgnoreReason() {
		t.Errorf("Parse(empty) = %+v, %v", cfg, err)
	}

	dir := t.TempDir()
	if _, ok := Find(dir); ok {
		t.Error("Find() found a config in an empty directory")
	}

	if err := os.WriteFile(filepath.Join(dir, ".migrail.yml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if path, ok := Find(dir); !ok || filepath.Base(path) != ".migrail.yml" {
		t.Errorf("Find() = %q, %v", path, ok)
	}
}
