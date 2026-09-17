package pretty

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const failOnNever = "never"

type Input struct {
	ToolVersion string
	Dialect     ir.Dialect
	DBVersion   ir.Version
	Migrations  []*ir.Migration
	Result      analyze.Result
	FailOn      string
	Elapsed     time.Duration
	Highlight   func(source string) []ir.Token
	Base        string
	ChangedOnly bool
}

type Options struct {
	Theme   theme.Theme
	Width   int
	Compact bool
	Link    func(path string, line int) string
}

type renderer struct {
	in   Input
	opts Options
	t    theme.Theme
}

func Write(w io.Writer, in Input, opts Options) error {
	if _, err := io.WriteString(w, Render(in, opts)); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}

func Render(in Input, opts Options) string {
	r := renderer{in: in, opts: opts, t: opts.Theme}

	var lines []string

	switch {
	case len(in.Result.Findings) == 0:
		lines = []string{r.clean()}
	case opts.Compact:
		lines = r.compact()
	default:
		lines = r.full()
	}

	return strings.Join(lines, "\n") + "\n"
}

func (r renderer) clean() string {
	t := r.t
	count := len(r.in.Migrations)

	if count == 0 && r.in.ChangedOnly {
		return t.Success.Render(t.Symbols.Success) + " " +
			t.Fg.Render("no changed migrations since "+r.in.Base) + r.dot() + t.Muted.Render(formatDuration(r.in.Elapsed))
	}

	return t.Success.Render(t.Symbols.Success) + " " +
		t.Fg.Render(fmt.Sprintf("%d %s checked", count, components.Plural(count, "migration", "migrations"))) +
		r.dot() + t.Fg.Render("no issues") +
		r.dot() + t.Muted.Render(formatDuration(r.in.Elapsed))
}

func (r renderer) dot() string {
	return r.t.Muted.Render(" " + r.t.Symbols.Dot + " ")
}

func (r renderer) compact() []string {
	t := r.t
	lines := make([]string, 0, len(r.in.Result.Findings)+1)

	for _, finding := range r.in.Result.Findings {
		lines = append(lines, t.Severity(finding.Severity).Render(t.SeveritySymbol(finding.Severity))+" "+
			t.Strong(t.Fg).Render(finding.RuleID)+" "+
			r.linked(t.Fg, finding.Location.Path, finding.Location.Span.Start.Line).
				Render(fmt.Sprintf("%s:%d", finding.Location.Path, finding.Location.Span.Start.Line))+"  "+
			components.HighlightNames(t, finding.Title))
	}

	counts := r.counts()
	parts := []string{}

	for _, entry := range counts.nonZero() {
		parts = append(parts, t.Severity(entry.severity).Render(entry.text))
	}

	parts = append(parts, t.Muted.Render(formatDuration(r.in.Elapsed)))
	lines = append(lines, strings.Join(parts, r.dot()))

	if failing := r.failingLine(); failing != "" {
		lines = append(lines, strings.TrimPrefix(failing, "  "))
	}

	return lines
}

func (r renderer) full() []string {
	lines := []string{r.header(), ""}

	for _, migration := range r.in.Migrations {
		findings := r.findingsFor(migration)
		if len(findings) == 0 {
			continue
		}

		lines = append(lines, r.fileHeader(migration), "")
		source := components.NewSource(migration.Source, r.highlight(migration.Source))

		for _, finding := range findings {
			lines = append(lines, r.finding(finding, source)...)
			lines = append(lines, "")
		}
	}

	return append(lines, r.summary()...)
}

func (r renderer) highlight(source string) []ir.Token {
	if r.in.Highlight == nil || !r.t.HasColor() {
		return nil
	}

	return r.in.Highlight(source)
}

func (r renderer) findingsFor(migration *ir.Migration) []ir.Finding {
	findings := []ir.Finding{}

	for _, finding := range r.in.Result.Findings {
		if finding.MigrationID == migration.ID {
			findings = append(findings, finding)
		}
	}

	return findings
}

func (r renderer) header() string {
	t := r.t
	files := len(r.in.Migrations)
	parts := []string{fmt.Sprintf("%s %s", r.in.Dialect, r.in.DBVersion)}

	if frameworks := r.frameworks(); frameworks != "" {
		parts = append(parts, frameworks)
	}

	if r.in.ChangedOnly {
		parts = append(parts, fmt.Sprintf("%d changed %s vs %s", files, components.Plural(files, "migration", "migrations"), r.in.Base))
	} else {
		parts = append(parts, fmt.Sprintf("%d %s", files, components.Plural(files, "file", "files")))
	}

	separator := " " + t.Symbols.Dot + " "

	return t.Muted.Render("migrail ") + t.Accent.Render(r.in.ToolVersion) + t.Muted.Render(separator+strings.Join(parts, separator))
}

func (r renderer) frameworks() string {
	seen := []string{}

	for _, migration := range r.in.Migrations {
		if migration.Framework != "" && migration.Framework != "sql" && !slices.Contains(seen, migration.Framework) {
			seen = append(seen, migration.Framework)
		}
	}

	return strings.Join(seen, ", ")
}

func (r renderer) fileHeader(migration *ir.Migration) string {
	t := r.t
	left := t.Accent.Render(t.Symbols.FileMarker) + " " +
		r.linked(t.Strong(t.Fg), migration.SourcePath, 1).Render(migration.SourcePath)

	if migration.Framework == "" || migration.Framework == "sql" {
		return left
	}

	return joinEnds(left, t.Muted.Render(migration.Framework), r.opts.Width)
}

func (r renderer) linked(style lipglossStyle, path string, line int) lipglossStyle {
	if r.opts.Link == nil {
		return style
	}

	if link := r.opts.Link(path, line); link != "" {
		return style.Hyperlink(link)
	}

	return style
}

func joinEnds(left, right string, width int) string {
	gap := width - visibleWidth(left) - visibleWidth(right)
	if gap < 2 {
		return left
	}

	return left + strings.Repeat(" ", gap) + right
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	return fmt.Sprintf("%.1fs", d.Seconds())
}
