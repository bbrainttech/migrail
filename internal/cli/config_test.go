package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProject(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func TestCheckUsesConfig(t *testing.T) {
	t.Parallel()

	root := writeProject(t, map[string]string{
		".migrail.yaml": `version: 1
db:
  version: "15"
git:
  check: all
rules:
  MR403: off
  create-index-non-concurrent: warning
policy:
  fail_on: warning
ignore:
  - path: "legacy/**"
    reason: legacy data
projects:
  - path: .
    migrations: db/migrations
  - path: legacy
    framework: sql
`,
		"db/migrations/001_index.sql": "CREATE INDEX idx_orders_status ON orders (status);\n",
		"legacy/001_old.sql":          "TRUNCATE sessions;\n",
	})

	run := runCLI(t, "", "check", "-f", "json", "--dir", root)
	if run.code != exitFindings {
		t.Fatalf("exit code = %d, want %d (stderr %q)", run.code, exitFindings, run.stderr)
	}

	var report struct {
		Database struct{ Version string } `json:"database"`
		Summary  struct {
			Warnings int `json:"warnings"`
			Errors   int `json:"errors"`
			Ignored  int `json:"ignored"`
		} `json:"summary"`
		Findings []struct {
			RuleID   string `json:"ruleId"`
			Severity string `json:"severity"`
		} `json:"findings"`
	}

	if err := json.Unmarshal([]byte(run.stdout), &report); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, run.stdout)
	}

	if report.Database.Version != "15" || report.Summary.Warnings != 1 || report.Summary.Errors != 0 || report.Summary.Ignored != 1 {
		t.Errorf("report = %+v", report)
	}

	overridden := runCLI(t, "", "check", "-f", "json", "--dir", root, "--fail-on", "error", "--db-version", "17")
	if overridden.code != exitOK || !strings.Contains(overridden.stdout, `"version": "17"`) {
		t.Errorf("flags should override config: code %d, stdout %s", overridden.code, overridden.stdout)
	}
}

func TestCheckReportsInvalidConfig(t *testing.T) {
	t.Parallel()

	root := writeProject(t, map[string]string{
		".migrail.yaml":         "version: 1\nrules:\n  MR10: off\n",
		"db/migrations/001.sql": "SELECT 1;\n",
	})

	run := runCLI(t, "", "check", "--dir", root)
	if run.code != exitUsage {
		t.Fatalf("exit code = %d, want %d", run.code, exitUsage)
	}

	for _, want := range []string{"Invalid config", `unknown rule "MR10"`, "Did you mean MR101?"} {
		if !strings.Contains(run.stderr, want) {
			t.Errorf("stderr is missing %q:\n%s", want, run.stderr)
		}
	}
}

func TestGitScopeUsesProjectRoot(t *testing.T) {
	t.Parallel()

	root := writeProject(t, map[string]string{
		"db/migrations/001_index.sql": "CREATE INDEX idx_orders_status ON orders (status);\n",
	})

	run := runCLI(t, "", "check", "-f", "json", "--db-version", "16", "--dir", root)
	if run.code != exitFindings {
		t.Fatalf("exit code = %d, want %d: migrations outside the current repository must still be checked (stderr %q)", run.code, exitFindings, run.stderr)
	}

	if !strings.Contains(run.stderr, "isn't a git repository") {
		t.Errorf("stderr = %q, want the not-a-repository notice for --dir", run.stderr)
	}
}
