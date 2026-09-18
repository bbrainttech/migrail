package cli

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		spec       string
		wantFormat string
		wantPath   string
		wantErr    string
	}{
		{spec: "migrail.sarif", wantFormat: formatSARIF, wantPath: "migrail.sarif"},
		{spec: "out/report.JSON", wantFormat: formatJSON, wantPath: "out/report.JSON"},
		{spec: "summary.md", wantFormat: formatMarkdown, wantPath: "summary.md"},
		{spec: "report.xml", wantFormat: formatJUnit, wantPath: "report.xml"},
		{spec: "findings.txt", wantFormat: formatPretty, wantPath: "findings.txt"},
		{spec: "github=annotations.log", wantFormat: formatGitHub, wantPath: "annotations.log"},
		{spec: "junit=a=b.xml", wantFormat: formatJUnit, wantPath: "a=b.xml"},
		{spec: "dir=x/report.sarif", wantFormat: formatSARIF, wantPath: "dir=x/report.sarif"},
		{spec: "report.html", wantErr: "can't tell the format"},
		{spec: "sarif=", wantErr: "add a file path"},
		{spec: "", wantErr: "pass a file path"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			t.Parallel()

			got, err := parseOutput(tt.spec)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("parseOutput(%q) error = %v, want %q", tt.spec, err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseOutput(%q): %v", tt.spec, err)
			}

			if got.format != tt.wantFormat || got.path != tt.wantPath {
				t.Errorf("parseOutput(%q) = %+v, want format %s path %s", tt.spec, got, tt.wantFormat, tt.wantPath)
			}
		})
	}
}

func TestCheckWritesOutputFiles(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "reports")
	files := map[string]string{
		"r.sarif":      "",
		"r.json":       "",
		"r.xml":        "",
		"summary.md":   "",
		"findings.txt": "",
	}

	args := []string{"check", "--db-version", "16", mr101Basic}
	for name := range files {
		args = append(args, "-o", filepath.Join(dir, name))
	}

	run := runCLI(t, "", args...)
	if run.code != exitFindings {
		t.Fatalf("exit code %d, want %d; stderr: %s", run.code, exitFindings, run.stderr)
	}

	if !strings.Contains(run.stdout, "MR101") {
		t.Errorf("stdout should still show the pretty report, got %q", run.stdout)
	}

	for name := range files {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		files[name] = string(data)
	}

	var sarifLog, jsonReport map[string]any
	if err := json.Unmarshal([]byte(files["r.sarif"]), &sarifLog); err != nil || sarifLog["version"] != "2.1.0" {
		t.Errorf("r.sarif is not a SARIF 2.1.0 log: %v", err)
	}

	if err := json.Unmarshal([]byte(files["r.json"]), &jsonReport); err != nil || jsonReport["schema"] == nil {
		t.Errorf("r.json is not a migrail JSON report: %v", err)
	}

	if err := xml.Unmarshal([]byte(files["r.xml"]), new(struct{})); err != nil {
		t.Errorf("r.xml is not valid XML: %v", err)
	}

	if !strings.HasPrefix(files["summary.md"], "## migrail") {
		t.Errorf("summary.md is not a markdown report: %q", files["summary.md"])
	}

	if strings.Contains(files["findings.txt"], "\x1b[") {
		t.Errorf("findings.txt contains color codes: %q", files["findings.txt"])
	}
}

func TestCheckInGitHubActions(t *testing.T) {
	summary := filepath.Join(t.TempDir(), "summary.md")
	t.Setenv(githubActionsEnv, "true")
	t.Setenv(githubSummaryEnv, summary)

	pretty := runCLI(t, "", "check", "--db-version", "16", mr101Basic)
	if !strings.Contains(pretty.stdout, "::error file=") {
		t.Errorf("pretty output in GitHub Actions should include annotations, got %q", pretty.stdout)
	}

	if strings.Contains(pretty.stdout, "migrail: 1 error") {
		t.Errorf("annotations after the pretty report should skip their count line, got %q", pretty.stdout)
	}

	jsonRun := runCLI(t, "", "check", "-f", formatJSON, "--db-version", "16", mr101Basic)
	if strings.Contains(jsonRun.stdout, "::error") {
		t.Errorf("JSON output must not include annotations, got %q", jsonRun.stdout)
	}

	data, err := os.ReadFile(summary)
	if err != nil {
		t.Fatalf("read job summary: %v", err)
	}

	if got := strings.Count(string(data), "## migrail"); got != 2 {
		t.Errorf("job summary should hold one report per run (2), got %d:\n%s", got, data)
	}
}

func TestPrettySummaryCountsOneStatement(t *testing.T) {
	t.Parallel()

	run := runCLI(t, "", "check", "--db-version", "16", "--color", "never", mr101Basic)

	if !strings.Contains(run.stdout, "1 file · 1 stmt ·") {
		t.Errorf("summary should say \"1 stmt\", got %q", run.stdout)
	}
}
