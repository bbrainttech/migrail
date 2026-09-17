package suppress

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	SourceInline = "inline"
	SourceConfig = "config"

	RuleIgnoreWithoutReason = "MR904"
	RuleUnusedIgnore        = "MR905"
)

var directivePattern = regexp.MustCompile(`--\s*migrail:(ignore-file|ignore)\b(.*)$`)

var reasonPattern = regexp.MustCompile(`reason\s*=\s*"([^"]*)"`)

type Directive struct {
	File      bool
	Rules     []string
	Reason    string
	Line      int
	Statement int
	used      bool
}

type ConfigIgnore struct {
	Path   string
	Rule   string
	Reason string
}

type Options struct {
	RequireReason bool
	Ignores       []ConfigIgnore
	Rules         []analyze.Rule
	Dialect       analyze.Dialect
	PathOf        func(*ir.Migration) string
}

func Parse(migration *ir.Migration) []*Directive {
	directives := []*Directive{}

	for number, line := range strings.Split(migration.Source, "\n") {
		match := directivePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}

		directive := &Directive{File: match[1] == "ignore-file", Line: number + 1, Statement: -1}
		arguments := match[2]

		if reason := reasonPattern.FindStringSubmatch(arguments); reason != nil {
			directive.Reason = strings.TrimSpace(reason[1])
			arguments = reasonPattern.ReplaceAllString(arguments, "")
		}

		for _, field := range strings.FieldsFunc(arguments, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			directive.Rules = append(directive.Rules, strings.TrimSpace(field))
		}

		if !directive.File {
			directive.Statement = nextStatement(migration, directive.Line)
		}

		directives = append(directives, directive)
	}

	return directives
}

func nextStatement(migration *ir.Migration, line int) int {
	for _, stmt := range migration.Statements {
		if stmt.Span.End.Line > line || stmt.Span.Start.Line > line {
			return stmt.Index
		}
	}

	return -1
}

func Apply(result analyze.Result, migrations []*ir.Migration, opts Options) analyze.Result {
	byID := map[string]*ir.Migration{}
	directives := map[string][]*Directive{}

	for _, migration := range migrations {
		byID[migration.ID] = migration
		directives[migration.ID] = Parse(migration)
	}

	for i := range result.Findings {
		finding := &result.Findings[i]
		migration := byID[finding.MigrationID]

		if suppression := match(finding, migration, directives[finding.MigrationID], opts); suppression != nil {
			finding.Suppressed = suppression
		}
	}

	for _, migration := range migrations {
		for _, directive := range directives[migration.ID] {
			result.Findings = append(result.Findings, diagnostics(migration, directive, opts)...)
		}
	}

	analyze.SortFindings(result.Findings)

	return result
}

func match(finding *ir.Finding, migration *ir.Migration, directives []*Directive, opts Options) *ir.Suppression {
	if migration == nil {
		return nil
	}

	migrationPath := migration.SourcePath
	if opts.PathOf != nil {
		migrationPath = opts.PathOf(migration)
	}

	for _, ignore := range opts.Ignores {
		if matchesRule(finding, []string{ignore.Rule}, true) && matchesPath(ignore.Path, migrationPath) {
			return &ir.Suppression{Reason: ignore.Reason, Source: SourceConfig}
		}
	}

	for _, directive := range directives {
		if !directive.File && (finding.Statement == nil || finding.Statement.Index != directive.Statement) {
			continue
		}

		if !matchesRule(finding, directive.Rules, false) {
			continue
		}

		directive.used = true

		if directive.Reason == "" && opts.RequireReason {
			continue
		}

		return &ir.Suppression{Reason: directive.Reason, Source: SourceInline, Line: directive.Line}
	}

	return nil
}

func matchesRule(finding *ir.Finding, rules []string, emptyMatchesAll bool) bool {
	if len(rules) == 0 || (len(rules) == 1 && rules[0] == "") {
		return emptyMatchesAll
	}

	return slices.ContainsFunc(rules, func(rule string) bool {
		return strings.EqualFold(rule, finding.RuleID) || rule == finding.Slug
	})
}

func matchesPath(pattern, name string) bool {
	if pattern == "" {
		return true
	}

	if ok, err := path.Match(pattern, name); err == nil && ok {
		return true
	}

	return globMatch(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func globMatch(pattern, parts []string) bool {
	if len(pattern) == 0 {
		return len(parts) == 0
	}

	if pattern[0] == "**" {
		for i := 0; i <= len(parts); i++ {
			if globMatch(pattern[1:], parts[i:]) {
				return true
			}
		}

		return false
	}

	if len(parts) == 0 {
		return false
	}

	ok, err := path.Match(pattern[0], parts[0])

	return err == nil && ok && globMatch(pattern[1:], parts[1:])
}

func diagnostics(migration *ir.Migration, directive *Directive, opts Options) []ir.Finding {
	findings := []ir.Finding{}
	span := ir.Span{Start: ir.Position{Line: directive.Line, Column: 1}, End: ir.Position{Line: directive.Line, Column: 1}}
	lines := ir.NewLineIndex(migration.Source)

	if start, end := lines.LineBounds(directive.Line); end > start {
		span = lines.Span(start, end)
	}

	if directive.Reason == "" && opts.RequireReason && directive.used {
		findings = append(findings, synthetic(migration, RuleIgnoreWithoutReason, opts, ir.Finding{
			Title:    "Ignore comment has no reason, so it doesn't apply",
			Why:      "Reviewers need to know why a finding is safe to ignore, so migrail requires a reason and still reports the finding.",
			Location: ir.SourceLoc{Span: span},
			Fix: &ir.Fix{
				Summary:   "Add a reason to the comment:",
				Framework: "sql",
				Steps:     []ir.FixStep{{Lang: "sql", Code: `-- migrail:ignore ` + strings.Join(directive.Rules, ",") + ` reason="why this is safe"`}},
			},
		}))
	}

	if !directive.used {
		findings = append(findings, synthetic(migration, RuleUnusedIgnore, opts, ir.Finding{
			Title:    "Ignore comment doesn't match any finding",
			Why:      "Nothing it names is reported here, so the comment only adds noise and can hide a future finding by accident.",
			Location: ir.SourceLoc{Span: span},
			Fix: &ir.Fix{
				Summary:   "Delete the comment, or move it directly above the statement and rule it's meant for.",
				Framework: "sql",
			},
		}))
	}

	return findings
}

func synthetic(migration *ir.Migration, ruleID string, opts Options, finding ir.Finding) ir.Finding {
	for _, rule := range opts.Rules {
		if rule.Meta().ID == ruleID {
			return analyze.Complete(opts.Dialect, rule.Meta(), migration, nil, finding)
		}
	}

	finding.RuleID = ruleID

	return finding
}
