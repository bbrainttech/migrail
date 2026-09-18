package junit

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

const (
	suiteName   = "migrail"
	errorType   = "RuleError"
	findingType = "MigrationFindings"
)

type Input struct {
	Migrations []*ir.Migration
	Result     analyze.Result
	FailOn     string
}

type testSuites struct {
	XMLName  xml.Name    `xml:"testsuites"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Suites   []testSuite `xml:"testsuite"`
}

type testSuite struct {
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Failures int        `xml:"failures,attr"`
	Errors   int        `xml:"errors,attr"`
	Skipped  int        `xml:"skipped,attr"`
	Cases    []testCase `xml:"testcase"`
}

type testCase struct {
	Name      string   `xml:"name,attr"`
	Classname string   `xml:"classname,attr"`
	File      string   `xml:"file,attr"`
	Failure   *problem `xml:"failure,omitempty"`
	Error     *problem `xml:"error,omitempty"`
	SystemOut *output  `xml:"system-out,omitempty"`
}

type problem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",cdata"`
}

type output struct {
	Text string `xml:",cdata"`
}

func Write(w io.Writer, in Input) error {
	data, err := xml.MarshalIndent(build(in), "", "  ")
	if err != nil {
		return fmt.Errorf("encode junit report: %w", err)
	}

	if _, err := io.WriteString(w, xml.Header+string(data)+"\n"); err != nil {
		return fmt.Errorf("write junit report: %w", err)
	}

	return nil
}

func build(in Input) testSuites {
	failOn := ir.Severity(in.FailOn)
	byMigration := map[string][]ir.Finding{}

	for _, finding := range in.Result.Findings {
		if finding.Suppressed == nil {
			byMigration[finding.MigrationID] = append(byMigration[finding.MigrationID], finding)
		}
	}

	errorsByMigration := map[string][]analyze.RuleError{}
	for _, e := range in.Result.RuleErrors {
		errorsByMigration[e.MigrationID] = append(errorsByMigration[e.MigrationID], e)
	}

	suite := testSuite{Name: suiteName, Cases: make([]testCase, 0, len(in.Migrations))}

	for _, migration := range in.Migrations {
		tc := buildCase(migration, byMigration[migration.ID], errorsByMigration[migration.ID], failOn)

		if tc.Failure != nil {
			suite.Failures++
		}

		if tc.Error != nil {
			suite.Errors++
		}

		suite.Cases = append(suite.Cases, tc)
	}

	suite.Tests = len(suite.Cases)

	return testSuites{Name: suiteName, Tests: suite.Tests, Failures: suite.Failures, Errors: suite.Errors, Suites: []testSuite{suite}}
}

func buildCase(migration *ir.Migration, findings []ir.Finding, ruleErrors []analyze.RuleError, failOn ir.Severity) testCase {
	tc := testCase{Name: migration.SourcePath, Classname: suiteName, File: migration.SourcePath}

	failing, other := split(findings, failOn)

	if len(failing) > 0 {
		tc.Failure = &problem{
			Message: fmt.Sprintf("%d %s at or above %s", len(failing), components.Plural(len(failing), "finding", "findings"), failOn),
			Type:    findingType,
			Body:    describe(failing),
		}
	}

	if len(other) > 0 {
		tc.SystemOut = &output{Text: describe(other)}
	}

	if len(ruleErrors) > 0 {
		lines := make([]string, 0, len(ruleErrors))
		for _, e := range ruleErrors {
			lines = append(lines, fmt.Sprintf("%s: %s", e.RuleID, e.Message))
		}

		tc.Error = &problem{
			Message: "rule checks failed, so findings may be incomplete",
			Type:    errorType,
			Body:    strings.Join(lines, "\n"),
		}
	}

	return tc
}

func split(findings []ir.Finding, failOn ir.Severity) (failing, other []ir.Finding) {
	for _, finding := range findings {
		if finding.Severity.FailsAt(failOn) {
			failing = append(failing, finding)
		} else {
			other = append(other, finding)
		}
	}

	return failing, other
}

func describe(findings []ir.Finding) string {
	blocks := make([]string, 0, len(findings))

	for _, finding := range findings {
		lines := []string{
			fmt.Sprintf("%s %s %s:%d %s", finding.Severity, finding.RuleID, finding.Location.Path, finding.Location.Span.Start.Line, finding.Title),
		}

		if finding.Why != "" {
			lines = append(lines, "why: "+finding.Why)
		}

		if finding.Fix != nil && finding.Fix.Summary != "" {
			lines = append(lines, "fix: "+finding.Fix.Summary)

			for _, step := range finding.Fix.Steps {
				if text := step.PlainText(); text != "" {
					lines = append(lines, text)
				}
			}
		}

		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	return strings.Join(blocks, "\n\n")
}
