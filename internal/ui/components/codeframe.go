package components

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const (
	tabWidth         = 4
	minGutterDigits  = 2
	maxSpanLines     = 5
	shownWhenCropped = 4
	frameIndent      = 2
)

type Source struct {
	Text   string
	Lines  *ir.LineIndex
	Tokens []ir.Token
}

func NewSource(text string, tokens []ir.Token) Source {
	sorted := append([]ir.Token(nil), tokens...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })

	return Source{Text: text, Lines: ir.NewLineIndex(text), Tokens: sorted}
}

type Frame struct {
	Theme    theme.Theme
	Width    int
	Severity ir.Severity
	Path     string
	Link     string
	Label    string
}

func GutterWidth(span ir.Span) int {
	lastLine := span.Start.Line + min(span.End.Line-span.Start.Line, maxSpanLines-1)

	return frameIndent + max(minGutterDigits, len(strconv.Itoa(lastLine)))
}

func (f Frame) Render(src Source, span ir.Span) []string {
	t := f.Theme
	gutter := GutterWidth(span)
	endsAtNextLineStart := span.End.Line == span.Start.Line+1 && span.End.Column == 1
	multiLine := span.End.Line > span.Start.Line && !endsAtNextLineStart

	location := fmt.Sprintf("%s:%d:%d", f.Path, span.Start.Line, span.Start.Column)

	locationStyle := t.Fg
	if f.Link != "" {
		locationStyle = locationStyle.Hyperlink(f.Link)
	}

	lines := []string{
		strings.Repeat(" ", gutter+1) + t.Muted.Render(t.Symbols.FirstBranch) + " " + locationStyle.Render(location),
	}

	if context := span.Start.Line - 1; context >= 1 && strings.TrimSpace(f.lineText(src, context)) != "" {
		lines = append(lines, f.codeLine(src, context, gutter, marker(multiLine, false, t, f.Severity)))
	}

	if !multiLine {
		lines = append(lines, f.codeLine(src, span.Start.Line, gutter, ""))

		return append(lines, f.caretLines(src, span, gutter)...)
	}

	return append(lines, f.multiLineSpan(src, span, gutter)...)
}

func (f Frame) multiLineSpan(src Source, span ir.Span, gutter int) []string {
	t := f.Theme
	last := span.End.Line

	if span.End.Column == 1 {
		last--
	}

	shown := last
	if last-span.Start.Line+1 > maxSpanLines {
		shown = span.Start.Line + shownWhenCropped - 1
	}

	lines := []string{}
	for line := span.Start.Line; line <= shown; line++ {
		lines = append(lines, f.codeLine(src, line, gutter, marker(true, true, t, f.Severity)))
	}

	prefix := f.pipePrefix(gutter)

	if shown < last {
		hidden := last - shown
		lines = append(lines, prefix+t.Severity(f.Severity).Render(t.Symbols.SpanMarker)+" "+
			t.Muted.Render(fmt.Sprintf("%s %d more %s", t.Symbols.Ellipsis, hidden, Plural(hidden, "line", "lines"))))
	}

	if f.Label != "" {
		lines = append(lines, prefix+"  "+t.Severity(f.Severity).Render(f.Label))
	}

	return lines
}

func marker(multiLine, inSpan bool, t theme.Theme, severity ir.Severity) string {
	switch {
	case !multiLine:
		return ""
	case inSpan:
		return t.Severity(severity).Render(t.Symbols.SpanMarker) + " "
	default:
		return "  "
	}
}

func (f Frame) pipePrefix(gutter int) string {
	return strings.Repeat(" ", gutter+1) + f.Theme.Muted.Render(f.Theme.Symbols.Pipe) + " "
}

func (f Frame) lineText(src Source, line int) string {
	start, end := src.Lines.LineBounds(line)

	return src.Text[start:end]
}

func (f Frame) codeLine(src Source, line, gutter int, mark string) string {
	t := f.Theme
	number := PadLeft(strconv.Itoa(line), gutter)
	prefix := t.Muted.Render(number) + " " + t.Muted.Render(t.Symbols.Pipe) + " " + mark
	available := f.Width - ansi.StringWidth(prefix)

	return prefix + Truncate(f.highlightLine(src, line), available, t)
}

func (f Frame) highlightLine(src Source, line int) string {
	start, end := src.Lines.LineBounds(line)

	var out strings.Builder

	position := start
	first := sort.Search(len(src.Tokens), func(i int) bool { return src.Tokens[i].End > start })

	for _, token := range src.Tokens[first:] {
		if token.Start >= end {
			break
		}

		tokenStart := max(token.Start, start)
		out.WriteString(f.Theme.Fg.Render(expandTabs(src.Text[position:tokenStart])))

		tokenEnd := min(token.End, end)
		out.WriteString(f.tokenStyle(token.Class).Render(expandTabs(src.Text[tokenStart:tokenEnd])))
		position = tokenEnd
	}

	out.WriteString(f.Theme.Fg.Render(expandTabs(src.Text[position:end])))

	return out.String()
}

func (f Frame) tokenStyle(class ir.TokenClass) lipgloss.Style {
	switch class {
	case ir.TokenKeyword:
		return f.Theme.Keyword
	case ir.TokenString:
		return f.Theme.String
	case ir.TokenComment:
		return f.Theme.Comment
	case ir.TokenPlain:
		return f.Theme.Fg
	default:
		return f.Theme.Fg
	}
}

func (f Frame) caretLines(src Source, span ir.Span, gutter int) []string {
	t := f.Theme
	lineStart, lineEnd := src.Lines.LineBounds(span.Start.Line)
	startOffset := min(max(span.Start.Offset, lineStart), lineEnd)
	endOffset := min(max(span.End.Offset, startOffset+1), lineEnd)

	leading := displayWidth(src.Text[lineStart:startOffset])
	length := max(1, displayWidth(src.Text[startOffset:endOffset]))

	prefix := f.pipePrefix(gutter)
	available := f.Width - ansi.StringWidth(prefix)
	severity := t.Severity(f.Severity)
	caret := strings.Repeat(" ", leading) + severity.Render(strings.Repeat(t.Symbols.Caret, length))

	if f.Label == "" {
		return []string{prefix + Truncate(caret, available, t)}
	}

	if leading+length+1+ansi.StringWidth(f.Label) <= available {
		return []string{prefix + caret + " " + severity.Render(f.Label)}
	}

	labelIndent := min(leading, max(0, available-ansi.StringWidth(f.Label)))
	lines := []string{prefix + Truncate(caret, available, t)}

	for _, line := range Wrap(f.Label, available-labelIndent) {
		lines = append(lines, prefix+strings.Repeat(" ", labelIndent)+severity.Render(line))
	}

	return lines
}

func expandTabs(text string) string {
	return strings.ReplaceAll(text, "\t", strings.Repeat(" ", tabWidth))
}

func displayWidth(text string) int {
	return ansi.StringWidth(expandTabs(text))
}

func HighlightedLines(t theme.Theme, src Source) []string {
	frame := Frame{Theme: t}
	lines := make([]string, 0, src.Lines.LineCount())

	for line := 1; line <= src.Lines.LineCount(); line++ {
		lines = append(lines, frame.highlightLine(src, line))
	}

	return lines
}
