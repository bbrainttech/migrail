package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	fixturesRoot      = "../../testdata/sql"
	minFixturesPerSet = 5
)

type expectedFinding struct {
	Line       int           `json:"line"`
	Column     int           `json:"column"`
	Severity   ir.Severity   `json:"severity"`
	Confidence ir.Confidence `json:"confidence"`
}

func TestRuleFixtures(t *testing.T) {
	t.Parallel()

	for _, rule := range All() {
		id := rule.Meta().ID

		t.Run(id, func(t *testing.T) {
			t.Parallel()

			expected := loadExpected(t, filepath.Join(fixturesRoot, id, "expected.json"))
			bad := fixtureFiles(t, id, "bad")
			good := fixtureFiles(t, id, "good")

			if len(bad) < minFixturesPerSet || len(good) < minFixturesPerSet {
				t.Fatalf("%s has %d bad and %d good fixtures, want at least %d of each", id, len(bad), len(good), minFixturesPerSet)
			}

			for _, file := range bad {
				want, ok := expected[file]
				if !ok || len(want) == 0 {
					t.Errorf("%s: missing expected findings in expected.json", file)

					continue
				}

				assertFindings(t, file, runFixture(t, id, file), want)
			}

			for _, file := range good {
				assertFindings(t, file, runFixture(t, id, file), []expectedFinding{})
			}

			for file := range expected {
				if !slices.Contains(bad, file) {
					t.Errorf("expected.json lists %s, which is not a bad fixture", file)
				}
			}
		})
	}
}

func loadExpected(t *testing.T, path string) map[string][]expectedFinding {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	expected := map[string][]expectedFinding{}
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return expected
}

func fixtureFiles(t *testing.T, id, kind string) []string {
	t.Helper()

	paths, err := filepath.Glob(filepath.Join(fixturesRoot, id, kind, "*.sql"))
	if err != nil {
		t.Fatalf("list fixtures: %v", err)
	}

	files := make([]string, 0, len(paths))
	for _, path := range paths {
		files = append(files, kind+"/"+filepath.Base(path))
	}

	return files
}

func runFixture(t *testing.T, id, file string) []ir.Finding {
	t.Helper()

	path := filepath.Join(fixturesRoot, id, filepath.FromSlash(file))

	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	dialect := pg.New()

	statements, err := dialect.Parse(string(source))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	migration := &ir.Migration{
		ID:         "sql:" + file,
		SourcePath: file,
		Source:     string(source),
		TxMode:     ir.TxModeNonTransactional,
		Statements: statements,
	}

	result, err := analyze.Run(context.Background(), dialect, []*ir.Migration{migration}, All(), analyze.Options{
		DBVersion: dialect.DefaultVersion(),
		Only:      []string{id},
	})
	if err != nil {
		t.Fatalf("analyze %s: %v", path, err)
	}

	if len(result.RuleErrors) > 0 {
		t.Fatalf("%s: rule errors: %+v", file, result.RuleErrors)
	}

	return result.Findings
}

func assertFindings(t *testing.T, file string, findings []ir.Finding, want []expectedFinding) {
	t.Helper()

	got := make([]expectedFinding, 0, len(findings))
	for _, finding := range findings {
		got = append(got, expectedFinding{
			Line:       finding.Location.Span.Start.Line,
			Column:     finding.Location.Span.Start.Column,
			Severity:   finding.Severity,
			Confidence: finding.Confidence,
		})

		assertFindingContent(t, file, finding)
	}

	if !slices.Equal(got, want) {
		t.Errorf("%s:\n got  %s\n want %s", file, describe(got), describe(want))
	}
}

func assertFindingContent(t *testing.T, file string, finding ir.Finding) {
	t.Helper()

	if finding.Title == "" || finding.Why == "" || finding.Fingerprint == "" {
		t.Errorf("%s: %s finding is missing a title, why or fingerprint: %+v", file, finding.RuleID, finding)
	}

	isDiagnostic := strings.HasPrefix(finding.RuleID, "MR9")
	if !isDiagnostic && (finding.Fix == nil || finding.Fix.Summary == "" || len(finding.Fix.Steps) == 0) {
		t.Errorf("%s: %s finding has no fix steps", file, finding.RuleID)
	}

	for _, step := range findingSteps(finding) {
		if step.Code != "" && !strings.HasPrefix(step.Code, "--") && !strings.HasSuffix(step.Code, ";") {
			t.Errorf("%s: %s fix code does not end with a semicolon: %q", file, finding.RuleID, step.Code)
		}
	}
}

func findingSteps(finding ir.Finding) []ir.FixStep {
	if finding.Fix == nil {
		return nil
	}

	return finding.Fix.Steps
}

func describe(findings []expectedFinding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, fmt.Sprintf("%d:%d %s/%s", f.Line, f.Column, f.Severity, f.Confidence))
	}

	return "[" + strings.Join(parts, ", ") + "]"
}
