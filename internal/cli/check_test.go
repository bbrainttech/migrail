package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixtures   = "../../testdata/sql"
	goldenDir  = "../../testdata/golden/json"
	mr101Basic = fixtures + "/MR101/bad/basic.sql"
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
		writeError(&stderr, err, code)
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
		{name: "error finding fails", args: []string{"check", mr101Basic}, wantCode: exitFindings, wantStderr: `1 finding at or above "error"`},
		{name: "clean file passes", args: []string{"check", fixtures + "/MR101/good/concurrently.sql"}, wantCode: exitOK},
		{name: "fail-on never", args: []string{"check", "--fail-on", "never", mr101Basic}, wantCode: exitOK},
		{name: "warning below threshold", args: []string{"check", fixtures + "/MR201/bad/to_varchar.sql"}, wantCode: exitOK},
		{name: "warning at threshold", args: []string{"check", "--fail-on", "warning", fixtures + "/MR201/bad/to_varchar.sql"}, wantCode: exitFindings},
		{name: "skip rule", args: []string{"check", "--skip-rule", "create-index-non-concurrent", mr101Basic}, wantCode: exitOK},
		{name: "stdin", stdin: "ALTER TABLE users RENAME COLUMN email TO mail;", args: []string{"check", "-"}, wantCode: exitFindings},
		{name: "no arguments", args: []string{"check"}, wantCode: exitUsage, wantStderr: "no migrations to check"},
		{name: "unsupported format", args: []string{"check", "-f", "sarif", mr101Basic}, wantCode: exitUsage, wantStderr: `unsupported format "sarif"`},
		{name: "invalid fail-on", args: []string{"check", "--fail-on", "fatal", mr101Basic}, wantCode: exitUsage},
		{name: "unsupported version", args: []string{"check", "--db-version", "11", mr101Basic}, wantCode: exitUsage, wantStderr: "PostgreSQL 12 to 18"},
		{name: "invalid version", args: []string{"check", "--db-version", "latest", mr101Basic}, wantCode: exitUsage},
		{name: "missing file", args: []string{"check", "nope.sql"}, wantCode: exitUsage, wantStderr: "read migration nope.sql"},
		{name: "directory", args: []string{"check", fixtures}, wantCode: exitUsage, wantStderr: "is a directory"},
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

			if tt.wantCode != exitUsage && !json.Valid([]byte(run.stdout)) {
				t.Errorf("stdout is not valid JSON: %q", run.stdout)
			}
		})
	}
}

func TestCheckVersionNotice(t *testing.T) {
	t.Parallel()

	withDefault := runCLI(t, "", "check", mr101Basic)
	if !strings.Contains(withDefault.stderr, "notice: assuming PostgreSQL 12") {
		t.Errorf("stderr = %q, want the default version notice", withDefault.stderr)
	}

	withVersion := runCLI(t, "", "check", "--db-version", "16", mr101Basic)
	if strings.Contains(withVersion.stderr, "notice:") {
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

			args := append([]string{"check", "--db-version", "16", "--fail-on", "never"}, tt.files...)
			run := runCLI(t, "", args...)

			assertGolden(t, filepath.Join(goldenDir, tt.name+".json"), run.stdout)
		})
	}
}

func assertGolden(t *testing.T, path, got string) {
	t.Helper()

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}

		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run make golden-update): %v", path, err)
	}

	if got != string(want) {
		t.Errorf("output differs from %s (run make golden-update and review the diff)\n got:\n%s", path, got)
	}
}
