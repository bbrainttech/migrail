package markdown

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const (
	indent     = "  "
	codeIndent = "    "
	columnGap  = 2
)

var (
	inlinePattern   = regexp.MustCompile("`[^`]+`|\\*\\*[^*]+\\*\\*|\\[[^\\]]+\\]\\([^)]+\\)")
	orderedItem     = regexp.MustCompile(`^\d+\. `)
	separatorCell   = regexp.MustCompile(`^:?-+:?$`)
	markdownLinkRef = regexp.MustCompile(`^\[([^\]]+)\]\(([^)]+)\)$`)
)

type Renderer struct {
	Theme      theme.Theme
	Width      int
	Highlight  func(source string) []ir.Token
	Hyperlinks bool
}

func (r Renderer) Render(source string) string {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	blocks := []string{}

	for i := 0; i < len(lines); {
		line := lines[i]

		switch {
		case strings.TrimSpace(line) == "":
			i++
		case strings.HasPrefix(line, "```"):
			block, next := r.codeBlock(lines, i)
			blocks, i = append(blocks, block), next
		case strings.HasPrefix(line, "# "):
			blocks, i = append(blocks, indent+r.Theme.Strong(r.Theme.Accent).Render(r.heading(line[2:]))), i+1
		case strings.HasPrefix(line, "## "):
			blocks, i = append(blocks, indent+r.Theme.Strong(r.Theme.Fg).Render(r.heading(line[3:]))), i+1
		case strings.HasPrefix(line, "|"):
			block, next := r.table(lines, i)
			blocks, i = append(blocks, block), next
		case strings.HasPrefix(line, "- ") || orderedItem.MatchString(line):
			block, next := r.list(lines, i)
			blocks, i = append(blocks, block), next
		default:
			block, next := r.paragraph(lines, i)
			blocks, i = append(blocks, block), next
		}
	}

	return strings.Join(blocks, "\n\n") + "\n"
}

func (r Renderer) heading(text string) string {
	return strings.ReplaceAll(text, "·", r.Theme.Symbols.Dot)
}

func (r Renderer) codeBlock(lines []string, start int) (string, int) {
	end := start + 1
	for end < len(lines) && !strings.HasPrefix(lines[end], "```") {
		end++
	}

	code := strings.Join(lines[start+1:end], "\n")

	var tokens []ir.Token
	if r.Highlight != nil && r.Theme.HasColor() {
		tokens = r.Highlight(code)
	}

	rendered := components.HighlightedLines(r.Theme, components.NewSource(code, tokens))
	for i, line := range rendered {
		rendered[i] = codeIndent + line
	}

	return strings.Join(rendered, "\n"), end + 1
}

func (r Renderer) paragraph(lines []string, start int) (string, int) {
	end := start
	parts := []string{}

	for end < len(lines) && strings.TrimSpace(lines[end]) != "" && !isBlockStart(lines[end]) {
		parts = append(parts, strings.TrimSpace(lines[end]))
		end++
	}

	return strings.Join(r.wrapInline(strings.Join(parts, " "), r.Width-len(indent), indent, indent), "\n"), end
}

func isBlockStart(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "```") || strings.HasPrefix(line, "|") ||
		strings.HasPrefix(line, "- ") || orderedItem.MatchString(line)
}

func (r Renderer) list(lines []string, start int) (string, int) {
	rendered := []string{}
	end := start

	for end < len(lines) {
		line := lines[end]

		marker := ""

		switch {
		case strings.HasPrefix(line, "- "):
			marker, line = r.Theme.Symbols.Bullet, line[2:]
		case orderedItem.MatchString(line):
			number := orderedItem.FindString(line)
			marker, line = strings.TrimSpace(number), line[len(number):]
		default:
			return strings.Join(rendered, "\n"), end
		}

		end++
		parts := []string{line}

		for end < len(lines) && strings.HasPrefix(lines[end], "  ") && strings.TrimSpace(lines[end]) != "" {
			parts = append(parts, strings.TrimSpace(lines[end]))
			end++
		}

		line = strings.Join(parts, " ")

		first := indent + r.Theme.Muted.Render(components.PadRight(marker, 3))
		hanging := indent + strings.Repeat(" ", 3)
		rendered = append(rendered, r.wrapInline(line, r.Width-len(hanging), first, hanging)...)
	}

	return strings.Join(rendered, "\n"), end
}

func (r Renderer) table(lines []string, start int) (string, int) {
	rows := [][]string{}
	end := start

	for end < len(lines) && strings.HasPrefix(lines[end], "|") {
		cells := splitRow(lines[end])
		end++

		if isSeparator(cells) {
			continue
		}

		rows = append(rows, cells)
	}

	if len(rows) == 0 {
		return "", end
	}

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i := range min(len(row), len(widths)) {
			widths[i] = max(widths[i], ansi.StringWidth(r.inline(row[i], r.Theme.Fg)))
		}
	}

	total := len(indent) + columnGap*(len(widths)-1)
	for _, width := range widths {
		total += width
	}

	if total > r.Width {
		return r.stackedTable(rows), end
	}

	rendered := make([]string, 0, len(rows))

	for rowIndex, row := range rows {
		style := r.Theme.Fg
		if rowIndex == 0 {
			style = r.Theme.Strong(r.Theme.Muted)
		}

		cells := make([]string, 0, len(row))
		for i := range min(len(row), len(widths)) {
			cells = append(cells, components.PadRight(r.inline(row[i], style), widths[i]))
		}

		rendered = append(rendered, strings.TrimRight(indent+strings.Join(cells, strings.Repeat(" ", columnGap)), " "))
	}

	return strings.Join(rendered, "\n"), end
}

func (r Renderer) stackedTable(rows [][]string) string {
	header := rows[0]
	blocks := []string{}

	for _, row := range rows[1:] {
		lines := []string{}

		for i := range min(len(row), len(header)) {
			label := r.Theme.Muted.Render(header[i] + ":")
			lines = append(lines, r.wrapInline(row[i], r.Width-len(indent)*2, indent+label+" ", indent+indent)...)
		}

		blocks = append(blocks, strings.Join(lines, "\n"))
	}

	return strings.Join(blocks, "\n\n")
}

func splitRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	cells := strings.Split(trimmed, "|")

	for i, cell := range cells {
		cells[i] = strings.TrimSpace(cell)
	}

	return cells
}

func isSeparator(cells []string) bool {
	for _, cell := range cells {
		if !separatorCell.MatchString(cell) {
			return false
		}
	}

	return true
}

type atom struct {
	text       string
	style      lipgloss.Style
	spaceAfter bool
}

func (r Renderer) atoms(text string, base lipgloss.Style) []atom {
	atoms := []atom{}
	position := 0

	addPlain := func(plain string, style lipgloss.Style) {
		words := strings.Split(plain, " ")
		for i, word := range words {
			if word == "" {
				if len(atoms) > 0 {
					atoms[len(atoms)-1].spaceAfter = true
				}

				continue
			}

			atoms = append(atoms, atom{text: word, style: style, spaceAfter: i < len(words)-1})
		}
	}

	for _, match := range inlinePattern.FindAllStringIndex(text, -1) {
		addPlain(text[position:match[0]], base)
		addPlain(r.inlineText(text[match[0]:match[1]]), r.inlineStyle(text[match[0]:match[1]], base))
		position = match[1]
	}

	addPlain(text[position:], base)

	return atoms
}

func (r Renderer) inlineText(token string) string {
	switch {
	case strings.HasPrefix(token, "`"):
		return strings.Trim(token, "`")
	case strings.HasPrefix(token, "**"):
		return strings.Trim(token, "*")
	default:
		parts := markdownLinkRef.FindStringSubmatch(token)
		if len(parts) != 3 {
			return token
		}

		if r.Hyperlinks {
			return parts[1]
		}

		return parts[1] + " (" + parts[2] + ")"
	}
}

func (r Renderer) inlineStyle(token string, base lipgloss.Style) lipgloss.Style {
	switch {
	case strings.HasPrefix(token, "`"):
		return r.Theme.Keyword
	case strings.HasPrefix(token, "**"):
		return r.Theme.Strong(base)
	default:
		parts := markdownLinkRef.FindStringSubmatch(token)
		if len(parts) == 3 && r.Hyperlinks {
			return r.Theme.Accent.Hyperlink(parts[2])
		}

		return r.Theme.Accent
	}
}

func (r Renderer) inline(text string, base lipgloss.Style) string {
	var out strings.Builder

	for _, a := range r.atoms(text, base) {
		out.WriteString(a.style.Render(a.text))

		if a.spaceAfter {
			out.WriteString(" ")
		}
	}

	return strings.TrimRight(out.String(), " ")
}

func (r Renderer) wrapInline(text string, width int, firstPrefix, restPrefix string) []string {
	lines := []string{}
	current := firstPrefix
	currentWidth := 0
	pendingSpace := false

	for _, a := range r.atoms(text, r.Theme.Fg) {
		atomWidth := ansi.StringWidth(a.text)
		space := 0

		if pendingSpace && currentWidth > 0 {
			space = 1
		}

		if currentWidth > 0 && currentWidth+space+atomWidth > width {
			lines = append(lines, current)
			current, currentWidth, space = restPrefix, 0, 0
		}

		current += strings.Repeat(" ", space) + a.style.Render(a.text)
		currentWidth += space + atomWidth
		pendingSpace = a.spaceAfter
	}

	return append(lines, current)
}
