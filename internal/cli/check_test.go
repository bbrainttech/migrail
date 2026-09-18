package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/ui/term"
)

const (
	fixtures        = "../../testdata/sql"
	goldenDir       = "../../testdata/golden/json"
	sarifGoldenDir  = "../../testdata/golden/sarif"
	githubGoldenDir = "../../testdata/golden/github"
	mr101Basic      = fixtures + "/MR101/bad/basic.sql"
	projects        = "../../testdata/projects"
)

type checkRun struct {
	code   int
	stdout string
	stderr string
}

func runCLI(t *testing.T, stdin string, args ...string) checkRun {
	t.Helper()

	var stdout, stderr bytes.Buffer

	root := newRootCommand(&stdout, &stderr)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)

	err := root.ExecuteContext(context.Background())
	code := exitOK

	if err != nil {
		code = exitCode(err)
		writeError(&stderr, err, code, term.Settings{})
	}

	return checkRun{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestCheckExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		stdin      string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{name: "error finding fails", args: []string{"check", "-f", "json", mr101Basic}, wantCode: exitFindings, wantStderr: `1 finding at or above "error"`},
		{name: "pretty summary replaces the stderr message", args: []string{"check", mr101Basic}, wantCode: exitFindings},
		{name: "clean file passes", args: []string{"check", fixtures + "/MR101/good/concurrently.sql"}, wantCode: exitOK},
		{name: "fail-on never", args: []string{"check", "--fail-on", "never", mr101Basic}, wantCode: exitOK},
		{name: "warning below threshold", args: []string{"check", fixtures + "/MR201/bad/to_varchar.sql"}, wantCode: exitOK},
		{name: "warning at threshold", args: []string{"check", "--fail-on", "warning", fixtures + "/MR201/bad/to_varchar.sql"}, wantCode: exitFindings},
		{name: "skip rule", args: []string{"check", "--skip-rule", "create-index-non-concurrent", mr101Basic}, wantCode: exitOK},
		{name: "stdin", stdin: "ALTER TABLE users RENAME COLUMN email TO mail;", args: []string{"check", "-"}, wantCode: exitFindings},
		{name: "no migrations under dir", args: []string{"check", "--dir", fixtures + "/MR101"}, wantCode: exitUsage, wantStderr: "No migrations found"},
		{name: "goose wraps migrations in a transaction", args: []string{"check", "-f", "json", projects + "/goose"}, wantCode: exitFindings},
		{name: "tables created earlier in the change set are new", args: []string{"check", "-f", "json", "--dir", projects + "/plain"}, wantCode: exitOK},
		{name: "forced framework", args: []string{"check", "-f", "json", "--framework", "goose", projects + "/goose/migrations/00003_no_tx.sql"}, wantCode: exitOK},
		{name: "unknown framework", args: []string{"check", "--framework", "rails", mr101Basic}, wantCode: exitUsage, wantStderr: `Unknown --framework "rails"`},
		{name: "unsupported format", args: []string{"check", "-f", "xml", mr101Basic}, wantCode: exitUsage, wantStderr: `Unsupported format "xml"`},
		{name: "invalid fail-on", args: []string{"check", "--fail-on", "fatal", mr101Basic}, wantCode: exitUsage},
		{name: "unsupported version", args: []string{"check", "--db-version", "11", mr101Basic}, wantCode: exitUsage, wantStderr: "PostgreSQL 12 to 18"},
		{name: "invalid version", args: []string{"check", "--db-version", "latest", mr101Basic}, wantCode: exitUsage},
		{name: "missing file", args: []string{"check", "nope.sql"}, wantCode: exitUsage, wantStderr: "Read migration nope.sql"},
		{name: "directory without migrations", args: []string{"check", fixtures}, wantCode: exitUsage, wantStderr: "No migrations found"},
		{name: "stdin mixed with files", args: []string{"check", "-", mr101Basic}, wantCode: exitUsage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			run := runCLI(t, tt.stdin, tt.args...)

			if run.code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", run.code, tt.wantCode, run.stderr)
			}

			if !strings.Contains(run.stderr, tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", run.stderr, tt.wantStderr)
			}

			if tt.wantCode != exitUsage && slices.Contains(tt.args, "json") && !json.Valid([]byte(run.stdout)) {
				t.Errorf("stdout is not valid JSON: %q", run.stdout)
			}
		})
	}
}

func TestCheckVersionNotice(t *testing.T) {
	t.Parallel()

	withDefault := runCLI(t, "", "check", mr101Basic)
	if !strings.Contains(withDefault.stderr, "notice assuming PostgreSQL 12") {
		t.Errorf("stderr = %q, want the default version notice", withDefault.stderr)
	}

	withVersion := runCLI(t, "", "check", "-f", "json", "--db-version", "16", mr101Basic)
	if strings.Contains(withVersion.stderr, "notice") {
		t.Errorf("stderr = %q, want no notice when --db-version is set", withVersion.stderr)
	}

	if !strings.Contains(withVersion.stdout, `"version": "16"`) {
		t.Errorf("stdout does not report database version 16")
	}
}

func TestCheckJSONGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files []string
	}{
		{name: "mr101_basic", files: []string{mr101Basic}},
		{name: "mixed", files: []string{
			fixtures + "/MR104/bad/inline_column_reference.sql",
			fixtures + "/MR107/bad/basic.sql",
			fixtures + "/MR201/bad/two_columns.sql",
			fixtures + "/MR302/bad/in_transaction.sql",
			fixtures + "/MR901/bad/misspelled_keyword.sql",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			args := append([]string{"check", "-f", "json", "--db-version", "16", "--fail-on", "never"}, tt.files...)
			run := runCLI(t, "", args...)

			golden.Assert(t, filepath.Join(goldenDir, tt.name+".json"), run.stdout)
		})
	}
}

func TestCheckSARIFGolden(t *testing.T) {
	t.Parallel()

	args := []string{
		"check", "-f", formatSARIF, "--db-version", "16", "--fail-on", failOnNever,
		mr101Basic,
		fixtures + "/MR302/bad/in_transaction.sql",
		fixtures + "/MR901/bad/misspelled_keyword.sql",
		fixtures + "/MR904/bad/no_reason.sql",
	}
	run := runCLI(t, "", args...)

	golden.Assert(t, filepath.Join(sarifGoldenDir, "mixed.sarif"), run.stdout)
}

func TestCheckGitHubGolden(t *testing.T) {
	t.Parallel()

	args := []string{
		"check", "-f", formatGitHub, "--db-version", "16", "--fail-on", failOnNever,
		mr101Basic,
		fixtures + "/MR302/bad/in_transaction.sql",
		fixtures + "/MR904/bad/no_reason.sql",
		fixtures + "/MR904/good/with_reason.sql",
	}
	run := runCLI(t, "", args...)

	golden.Assert(t, filepath.Join(githubGoldenDir, "mixed.txt"), run.stdout)
}
