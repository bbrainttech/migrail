package suppress

import (
	"context"
	"testing"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/rules"
)

func check(t *testing.T, source string, opts Options, analyzeOpts analyze.Options) []ir.Finding {
	t.Helper()

	dialect := pg.New()

	statements, err := dialect.Parse(source)
	if err != nil {
		t.Fatal(err)
	}

	migration := &ir.Migration{ID: "sql:m.sql", SourcePath: "db/m.sql", Source: source, Statements: statements}
	all := rules.All()

	analyzeOpts.DBVersion = ir.Version{Major: 16}

	result, err := analyze.Run(context.Background(), dialect, []*ir.Migration{migration}, all, analyzeOpts)
	if err != nil {
		t.Fatal(err)
	}

	opts.Rules = all
	opts.Dialect = dialect

	return Apply(result, []*ir.Migration{migration}, opts).Findings
}

func count(findings []ir.Finding, ruleID string, suppressed bool) int {
	n := 0

	for _, finding := range findings {
		if finding.RuleID == ruleID && (finding.Suppressed != nil) == suppressed {
			n++
		}
	}

	return n
}

func TestEveryMatchingIgnoreCountsAsUsed(t *testing.T) {
	t.Parallel()

	source := "-- migrail:ignore-file MR101 reason=\"empty table\"\n" +
		"SET lock_timeout = '5s';\n" +
		"-- migrail:ignore MR101 reason=\"also covered\"\n" +
		"CREATE INDEX idx ON orders (status);\n"

	findings := check(t, source, Options{
		RequireReason: true,
		Ignores:       []ConfigIgnore{{Path: "db/**", Rule: "MR101", Reason: "config"}},
	}, analyze.Options{})

	if count(findings, "MR101", true) != 1 || count(findings, "MR905", false) != 0 {
		t.Errorf("findings = %+v", findings)
	}
}

func TestUnusedIgnoreForInactiveRuleIsNotReported(t *testing.T) {
	t.Parallel()

	source := "-- migrail:ignore MR102 reason=\"drop is fine\"\nDROP INDEX idx_old;\n"
	active := func(id string) bool { return id != "MR102" }

	findings := check(t, source, Options{RequireReason: true, Active: active}, analyze.Options{Skip: []string{"MR102"}})
	if count(findings, "MR905", false) != 0 {
		t.Errorf("MR905 reported for a skipped rule: %+v", findings)
	}

	unknown := check(t, "-- migrail:ignore MR999 reason=\"typo\"\nSELECT 1;\n", Options{RequireReason: true, Active: active}, analyze.Options{})
	if count(unknown, "MR905", false) != 1 {
		t.Errorf("MR905 not reported for an unknown rule: %+v", unknown)
	}
}

func TestStatementlessFindingsHaveDistinctFingerprints(t *testing.T) {
	t.Parallel()

	source := "-- migrail:ignore MR101 reason=\"a\"\nSELECT 1;\n-- migrail:ignore MR101 reason=\"b\"\nSELECT 2;\n"
	findings := check(t, source, Options{RequireReason: true}, analyze.Options{})

	fingerprints := map[string]bool{}

	for _, finding := range findings {
		if finding.RuleID == "MR905" {
			fingerprints[finding.Fingerprint] = true
		}
	}

	if len(fingerprints) != 2 {
		t.Errorf("want 2 distinct MR905 fingerprints, got %v", fingerprints)
	}
}
