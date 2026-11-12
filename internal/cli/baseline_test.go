package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/baseline"
)

const gooseIndexMigration = "-- +goose Up\nSET lock_timeout = '5s';\nCREATE INDEX idx_%s ON orders (id);\n"

func writeGooseMigration(t *testing.T, root, name string) {
	t.Helper()

	dir := filepath.Join(root, "migrations")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	content := strings.ReplaceAll(gooseIndexMigration, "%s", strings.TrimSuffix(name, ".sql"))
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func jsonSummary(t *testing.T, stdout string) (errors, ignored int) {
	t.Helper()

	var report struct {
		Summary struct {
			Errors  int `json:"errors"`
			Ignored int `json:"ignored"`
		} `json:"summary"`
	}

	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("parse JSON report: %v\n%s", err, stdout)
	}

	return report.Summary.Errors, report.Summary.Ignored
}

func TestBaselineHidesOnlyRecordedFindings(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGooseMigration(t, root, "00001_first.sql")

	created := runCLI(t, "", "baseline", "--dir", root, "--db-version", "16")
	if created.code != exitOK || !strings.Contains(created.stdout, "Recorded 1 finding from 1 migration") {
		t.Fatalf("baseline: code %d, stdout %q, stderr %q", created.code, created.stdout, created.stderr)
	}

	if _, err := os.Stat(filepath.Join(root, baseline.FileName)); err != nil {
		t.Fatalf("baseline file wasn't written: %v", err)
	}

	clean := runCLI(t, "", "check", "--dir", root, "--all", "--db-version", "16", "-f", "json")
	if errors, ignored := jsonSummary(t, clean.stdout); clean.code != exitOK || errors != 0 || ignored != 1 {
		t.Fatalf("recorded finding: code %d, errors %d, ignored %d", clean.code, errors, ignored)
	}

	writeGooseMigration(t, root, "00002_second.sql")

	withNew := runCLI(t, "", "check", "--dir", root, "--all", "--db-version", "16", "-f", "json")
	if errors, ignored := jsonSummary(t, withNew.stdout); withNew.code != exitFindings || errors != 1 || ignored != 1 {
		t.Fatalf("new finding: code %d, errors %d, ignored %d", withNew.code, errors, ignored)
	}

	everything := runCLI(t, "", "check", "--dir", root, "--all", "--db-version", "16", "--no-baseline", "-f", "json")
	if errors, ignored := jsonSummary(t, everything.stdout); errors != 2 || ignored != 0 {
		t.Fatalf("--no-baseline: errors %d, ignored %d", errors, ignored)
	}
}

func TestBaselineMatchesFromASubdirectory(t *testing.T) { //nolint:paralleltest // t.Chdir changes the working directory of the whole process
	root := t.TempDir()
	writeGooseMigration(t, root, "00001_first.sql")

	if created := runCLI(t, "", "baseline", "--dir", root, "--db-version", "16"); created.code != exitOK {
		t.Fatalf("baseline: code %d, stderr %q", created.code, created.stderr)
	}

	t.Chdir(filepath.Join(root, "migrations"))

	result := runCLI(t, "", "check", "--dir", root, "--all", "--db-version", "16", "-f", "json")
	if errors, ignored := jsonSummary(t, result.stdout); result.code != exitOK || errors != 0 || ignored != 1 {
		t.Fatalf("from a subdirectory: code %d, errors %d, ignored %d", result.code, errors, ignored)
	}
}

func TestBaselineFlagNamingMissingFileFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeGooseMigration(t, root, "00001_first.sql")

	result := runCLI(t, "", "check", "--dir", root, "--all", "--baseline", filepath.Join(root, "missing.json"))
	if result.code != exitUsage || !strings.Contains(result.stderr, "run migrail baseline") {
		t.Errorf("code %d, stderr %q, want a usage error naming migrail baseline", result.code, result.stderr)
	}
}
