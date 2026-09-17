package analyze

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/bbrainttech/migrail/internal/ir"
)

type Rule interface {
	Meta() ir.RuleMeta
	Check(c *Context, stmt *ir.Statement) []ir.Finding
}

type MigrationRule interface {
	CheckMigration(c *Context, migration *ir.Migration) []ir.Finding
}

type Dialect interface {
	Name() ir.Dialect
	Fingerprint(stmt *ir.Statement) string
}

type Options struct {
	DBVersion ir.Version
	Only      []string
	Skip      []string
}

type RuleError struct {
	RuleID      string
	MigrationID string
	Statement   int
	Message     string
}

type Result struct {
	Findings   []ir.Finding
	RuleErrors []RuleError
	Statements int
	RulesRun   int
}

func Run(ctx context.Context, dialect Dialect, migrations []*ir.Migration, rules []Rule, opts Options) (Result, error) {
	active := ActiveRules(rules, dialect.Name(), opts)
	state := newTracker()
	result := Result{Findings: []ir.Finding{}, RuleErrors: []RuleError{}, RulesRun: len(active)}

	for _, migration := range migrations {
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("analyze migrations: %w", err)
		}

		checkMigration(dialect, migration, active, opts, state, &result)
	}

	result.Findings = dedupe(result.Findings)
	SortFindings(result.Findings)

	return result, nil
}

func checkMigration(dialect Dialect, migration *ir.Migration, rules []Rule, opts Options, state *tracker, result *Result) {
	state.startMigration(migration)

	analysisContext := &Context{
		Dialect:   dialect.Name(),
		DBVersion: opts.DBVersion,
		Migration: migration,
		state:     state,
	}

	for _, rule := range rules {
		migrationRule, ok := rule.(MigrationRule)
		if !ok {
			continue
		}

		findings, err := safeRun(rule, func() []ir.Finding { return migrationRule.CheckMigration(analysisContext, migration) })
		if err != nil {
			result.RuleErrors = append(result.RuleErrors, RuleError{RuleID: rule.Meta().ID, MigrationID: migration.ID, Statement: -1, Message: err.Error()})

			continue
		}

		for _, finding := range findings {
			result.Findings = append(result.Findings, Complete(dialect, rule.Meta(), migration, nil, finding))
		}
	}

	for _, stmt := range migration.Statements {
		stmt.InTx = state.inTransaction(migration)
		result.Statements++

		for _, rule := range rules {
			findings, err := safeCheck(rule, analysisContext, stmt)
			if err != nil {
				result.RuleErrors = append(result.RuleErrors, RuleError{
					RuleID:      rule.Meta().ID,
					MigrationID: migration.ID,
					Statement:   stmt.Index,
					Message:     err.Error(),
				})

				continue
			}

			for _, finding := range findings {
				result.Findings = append(result.Findings, Complete(dialect, rule.Meta(), migration, stmt, finding))
			}
		}

		state.apply(migration, stmt)
	}
}

func safeCheck(rule Rule, c *Context, stmt *ir.Statement) ([]ir.Finding, error) {
	return safeRun(rule, func() []ir.Finding { return rule.Check(c, stmt) })
}

func safeRun(rule Rule, check func() []ir.Finding) (findings []ir.Finding, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("rule %s panicked: %v", rule.Meta().ID, recovered)
		}
	}()

	return check(), nil
}

func ActiveRules(rules []Rule, dialect ir.Dialect, opts Options) []Rule {
	active := []Rule{}

	for _, rule := range rules {
		meta := rule.Meta()

		selected := meta.DefaultEnabled
		if len(opts.Only) > 0 {
			selected = matchesRule(meta, opts.Only)
		}

		if !selected || matchesRule(meta, opts.Skip) || !supportsDialect(meta, dialect) || !supportsVersion(meta, opts.DBVersion) {
			continue
		}

		active = append(active, rule)
	}

	return active
}

func matchesRule(meta ir.RuleMeta, names []string) bool {
	return slices.Contains(names, meta.ID) || slices.Contains(names, meta.Slug)
}

func supportsDialect(meta ir.RuleMeta, dialect ir.Dialect) bool {
	return len(meta.Dialects) == 0 || slices.Contains(meta.Dialects, dialect)
}

func supportsVersion(meta ir.RuleMeta, version ir.Version) bool {
	if meta.MinVersion != nil && version.Compare(*meta.MinVersion) < 0 {
		return false
	}

	return meta.MaxVersion == nil || version.Compare(*meta.MaxVersion) <= 0
}

func Complete(dialect Dialect, meta ir.RuleMeta, migration *ir.Migration, stmt *ir.Statement, finding ir.Finding) ir.Finding {
	finding.RuleID = meta.ID
	finding.Slug = meta.Slug
	finding.MigrationID = migration.ID
	finding.Statement = stmt

	if finding.Severity == "" {
		finding.Severity = meta.DefaultSeverity
	}

	if finding.Confidence == "" {
		finding.Confidence = ir.ConfidenceDefinite
	}

	statementFingerprint := ""

	if stmt != nil {
		statementFingerprint = dialect.Fingerprint(stmt)

		if finding.Location.Span == (ir.Span{}) {
			finding.Location.Span = stmt.Span
		}
	}

	if finding.Location.Span == (ir.Span{}) {
		finding.Location.Span = ir.Span{Start: ir.Position{Line: 1, Column: 1}, End: ir.Position{Line: 1, Column: 1}}
	}

	if stmt == nil {
		statementFingerprint = fmt.Sprintf("line:%d", finding.Location.Span.Start.Line)
	}

	finding.Location.Path = migration.SourcePath

	sum := sha256.Sum256([]byte(meta.ID + "\x00" + migration.ID + "\x00" + statementFingerprint))
	finding.Fingerprint = hex.EncodeToString(sum[:])

	return finding
}

func dedupe(findings []ir.Finding) []ir.Finding {
	seen := map[string]struct{}{}
	unique := make([]ir.Finding, 0, len(findings))

	for _, finding := range findings {
		statementIndex := -1
		if finding.Statement != nil {
			statementIndex = finding.Statement.Index
		}

		key := fmt.Sprintf(
			"%s|%s|%d|%d|%s",
			finding.RuleID,
			finding.MigrationID,
			statementIndex,
			finding.Location.Span.Start.Offset,
			finding.Title,
		)
		if _, ok := seen[key]; ok {
			continue
		}

		seen[key] = struct{}{}

		unique = append(unique, finding)
	}

	return unique
}

func SortFindings(findings []ir.Finding) {
	slices.SortStableFunc(findings, func(a, b ir.Finding) int {
		return cmp.Or(
			cmp.Compare(a.Location.Path, b.Location.Path),
			cmp.Compare(a.Location.Span.Start.Offset, b.Location.Span.Start.Offset),
			cmp.Compare(a.RuleID, b.RuleID),
		)
	})
}
