package components

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/bbrainttech/migrail/internal/ui/theme"
)

var quotedName = regexp.MustCompile(`"[^"\s]+"`)

func Wrap(text string, width int) []string {
	width = max(width, 1)
	lines := []string{}

	for paragraph := range strings.SplitSeq(text, "\n") {
		lines = append(lines, wrapParagraph(paragraph, width)...)
	}

	return lines
}

func wrapParagraph(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{}
	current := words[0]

	for _, word := range words[1:] {
		if ansi.StringWidth(current)+1+ansi.StringWidth(word) > width {
			lines = append(lines, current)
			current = word

			continue
		}

		current += " " + word
	}

	return append(lines, current)
}

func HighlightNames(t theme.Theme, text string) string {
	var out strings.Builder

	last := 0
	for _, match := range quotedName.FindAllStringIndex(text, -1) {
		out.WriteString(t.Fg.Render(text[last:match[0]]))
		out.WriteString(t.Accent.Render(text[match[0]:match[1]]))
		last = match[1]
	}

	out.WriteString(t.Fg.Render(text[last:]))

	return out.String()
}

func PadRight(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func PadLeft(text string, width int) string {
	return strings.Repeat(" ", max(0, width-ansi.StringWidth(text))) + text
}

func Truncate(text string, width int, t theme.Theme) string {
	if ansi.StringWidth(text) <= width {
		return text
	}

	ellipsis := t.Symbols.Ellipsis

	return ansi.Truncate(text, max(0, width-ansi.StringWidth(ellipsis)), "") + t.Muted.Render(ellipsis)
}

func Plural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}

	return plural
}
