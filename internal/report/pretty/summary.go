package pretty

import (
	"fmt"
	"strings"

	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

type severityCount struct {
	severity ir.Severity
	count    int
	text     string
}

type severityCounts []severityCount

func (c severityCounts) nonZero() severityCounts {
	kept := severityCounts{}

	for _, entry := range c {
		if entry.count > 0 {
			kept = append(kept, entry)
		}
	}

	return kept
}

func (r renderer) counts() severityCounts {
	totals := map[ir.Severity]int{}
	for _, finding := range r.in.Result.Findings {
		totals[finding.Severity]++
	}

	entries := []struct {
		severity         ir.Severity
		singular, plural string
	}{
		{severity: ir.SeverityError, singular: "error", plural: "errors"},
		{severity: ir.SeverityWarning, singular: "warning", plural: "warnings"},
		{severity: ir.SeverityNotice, singular: "notice", plural: "notices"},
	}

	counts := make(severityCounts, 0, len(entries))

	for _, entry := range entries {
		count := totals[entry.severity]
		counts = append(counts, severityCount{
			severity: entry.severity,
			count:    count,
			text:     fmt.Sprintf("%d %s", count, components.Plural(count, entry.singular, entry.plural)),
		})
	}

	return counts
}

func (r renderer) summary() []string {
	t := r.t
	parts := []string{}

	for _, entry := range r.counts() {
		text := t.SeveritySymbol(entry.severity) + " " + entry.text
		if entry.count == 0 {
			parts = append(parts, t.Muted.Render(text))

			continue
		}

		parts = append(parts, t.Severity(entry.severity).Render(text))
	}

	files := len(r.in.Migrations)
	left := "  " + strings.Join(parts, "  ")
	right := t.Muted.Render(fmt.Sprintf("%d %s %s %d stmts %s %s",
		files, components.Plural(files, "file", "files"), t.Symbols.Dot,
		r.in.Result.Statements, t.Symbols.Dot, formatDuration(r.in.Elapsed)))

	lines := []string{t.Subtle.Render(strings.Repeat(t.Symbols.Rule, r.opts.Width))}

	if visibleWidth(left)+2+visibleWidth(right) <= r.opts.Width {
		lines = append(lines, joinEnds(left, right, r.opts.Width))
	} else {
		lines = append(lines, left, "  "+right)
	}

	if failing := r.failing(); failing > 0 {
		lines = append(lines, "  "+t.Strong(t.Error).Render("Failing:")+" "+
			t.Fg.Render(fmt.Sprintf("%d %s at or above %q", failing, components.Plural(failing, "finding", "findings"), r.in.FailOn)))
	}

	return lines
}

func (r renderer) failing() int {
	if r.in.FailOn == "" || r.in.FailOn == failOnNever {
		return 0
	}

	count := 0

	for _, finding := range r.in.Result.Findings {
		if finding.Severity.AtLeast(ir.Severity(r.in.FailOn)) {
			count++
		}
	}

	return count
}
