package pretty

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

type lipglossStyle = lipgloss.Style

const (
	labelWidth     = 8
	branchWidth    = 3
	stepCodeIndent = 3
)

type label struct {
	name  string
	style lipgloss.Style
	lines []string
}

func visibleWidth(text string) int {
	return ansi.StringWidth(text)
}

func (r renderer) finding(finding ir.Finding, source components.Source) []string {
	t := r.t
	severity := t.Severity(finding.Severity)

	prefix := "  " + severity.Render(t.SeveritySymbol(finding.Severity)) + " " +
		t.Strong(severity).Render(string(finding.Severity)) + " " +
		t.Strong(t.Fg).Render(finding.RuleID) + " "

	titleLines := components.Wrap(finding.Title, r.opts.Width-visibleWidth(prefix))
	lines := []string{prefix + components.HighlightNames(t, titleLines[0])}

	for _, line := range titleLines[1:] {
		lines = append(lines, strings.Repeat(" ", visibleWidth(prefix))+components.HighlightNames(t, line))
	}

	span := finding.Location.Span
	gutter := components.GutterWidth(span)

	frame := components.Frame{
		Theme:    t,
		Width:    r.opts.Width,
		Severity: finding.Severity,
		Path:     finding.Location.Path,
		Label:    finding.Label,
	}

	if r.opts.Link != nil {
		frame.Link = r.opts.Link(finding.Location.Path, span.Start.Line)
	}

	lines = append(lines, "")
	lines = append(lines, frame.Render(source, span)...)
	lines = append(lines, strings.Repeat(" ", gutter+1)+t.Muted.Render(t.Symbols.Pipe))
	lines = append(lines, r.labels(finding, gutter)...)

	if finding.Fix != nil {
		lines = append(lines, r.fixSteps(finding.Fix, gutter)...)
	}

	return lines
}

func (r renderer) valueIndent(gutter int) int {
	return gutter + 1 + branchWidth + labelWidth
}

func (r renderer) labels(finding ir.Finding, gutter int) []string {
	t := r.t
	width := r.opts.Width - r.valueIndent(gutter)
	labels := []label{}

	if finding.Lock != nil {
		labels = append(labels, label{name: "lock", style: t.Muted, lines: r.lockLines(finding.Lock, width)})
	}

	if finding.Label == "" && finding.Why != "" {
		labels = append(labels, label{name: "why", style: t.Muted, lines: plainLines(t.Fg, components.Wrap(finding.Why, width))})
	}

	if finding.Note != "" {
		labels = append(labels, label{name: "note", style: t.Muted, lines: plainLines(t.Fg, components.Wrap(finding.Note, width))})
	}

	if finding.Fix != nil && finding.Fix.Summary != "" {
		labels = append(labels, label{name: "fix", style: t.Success, lines: plainLines(t.Fg, components.Wrap(finding.Fix.Summary, width))})
	}

	lines := []string{}
	indent := strings.Repeat(" ", gutter+1)

	for i, entry := range labels {
		last := i == len(labels)-1
		branch := t.Symbols.Branch
		continuation := t.Muted.Render(t.Symbols.Pipe) + strings.Repeat(" ", branchWidth+labelWidth-1)

		if last {
			branch = t.Symbols.LastBranch
			continuation = strings.Repeat(" ", branchWidth+labelWidth)
		}

		lines = append(lines, indent+t.Muted.Render(branch)+" "+entry.style.Render(components.PadRight(entry.name, labelWidth))+entry.lines[0])

		for _, line := range entry.lines[1:] {
			lines = append(lines, indent+continuation+line)
		}
	}

	return lines
}

func plainLines(style lipgloss.Style, lines []string) []string {
	styled := make([]string, 0, len(lines))
	for _, line := range lines {
		styled = append(styled, style.Render(line))
	}

	return styled
}

func (r renderer) lockLines(lock *ir.LockImpact, width int) []string {
	t := r.t
	tables := make([]string, 0, len(lock.Tables))

	for _, table := range lock.Tables {
		tables = append(tables, t.Accent.Render(table))
	}

	text := t.Fg.Render(lock.Mode+" on ") + strings.Join(tables, t.Fg.Render(", ")) + r.dot() + t.Fg.Render(blockedText(lock.Blocks))

	if lock.Rewrite {
		text += r.dot() + t.Fg.Render("rewrites the table")
	}

	if visibleWidth(text) <= width {
		return []string{text}
	}

	return plainLines(t.Fg, components.Wrap(ansi.Strip(text), width))
}

func blockedText(blocks []string) string {
	switch {
	case len(blocks) == 0:
		return "doesn't block reads or writes"
	case len(blocks) == 4:
		return "blocks reads and writes"
	default:
		return "blocks " + strings.Join(blocks, ", ")
	}
}

func (r renderer) fixSteps(fix *ir.Fix, gutter int) []string {
	t := r.t
	indent := strings.Repeat(" ", r.valueIndent(gutter))
	width := r.opts.Width - r.valueIndent(gutter)

	if len(fix.Steps) == 1 && fix.Steps[0].Title == "" {
		return append([]string{""}, r.codeLines(fix.Steps[0].Code, indent)...)
	}

	hasCode := false

	for _, step := range fix.Steps {
		hasCode = hasCode || step.Code != ""
	}

	lines := []string{}

	for i, step := range fix.Steps {
		if hasCode {
			lines = append(lines, "")
		}

		number := t.Muted.Render(components.PadRight(strconv.Itoa(i+1), stepCodeIndent))
		titleLines := components.Wrap(step.Title, width-stepCodeIndent)
		lines = append(lines, indent+number+t.Fg.Render(titleLines[0]))

		for _, line := range titleLines[1:] {
			lines = append(lines, indent+strings.Repeat(" ", stepCodeIndent)+t.Fg.Render(line))
		}

		if step.Code != "" {
			lines = append(lines, r.codeLines(step.Code, indent+strings.Repeat(" ", stepCodeIndent))...)
		}
	}

	return lines
}

func (r renderer) codeLines(code, indent string) []string {
	source := components.NewSource(code, r.highlight(code))
	lines := []string{}

	for _, line := range components.HighlightedLines(r.t, source) {
		lines = append(lines, indent+line)
	}

	return lines
}
