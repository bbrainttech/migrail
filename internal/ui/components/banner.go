package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const pillar = "██║"

var letterI = []string{"██╗", pillar, pillar, pillar, pillar, "╚═╝"}

var letters = [][]string{
	{"███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"},
	letterI,
	{" ██████╗ ", "██╔════╝ ", "██║  ███╗", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	{"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	{" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	letterI,
	{"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
}

var tagline = []string{
	"catch dangerous migrations",
	"before they reach production",
}

const (
	bannerMinimum = 60
	bannerIndent  = " "
	railTieEvery  = 4
	solidBlock    = '█'
)

func Banner(t theme.Theme, width int) string {
	if width < bannerMinimum {
		return t.Strong(t.Accent).Render("migrail") + "\n"
	}

	rows := wordmarkRows()
	artWidth := len([]rune(rows[0]))
	lines := make([]string, 0, len(rows)+len(tagline)+1)

	for _, row := range rows {
		rendered := renderWordmarkRow(t, row, artWidth)
		if strings.TrimSpace(rendered) == "" {
			continue
		}

		lines = append(lines, bannerIndent+rendered)
	}

	lines = append(lines, rail(t, artWidth+len(bannerIndent)))

	for _, row := range tagline {
		lines = append(lines, bannerIndent+t.Muted.Render(row))
	}

	return strings.Join(lines, "\n") + "\n"
}

func wordmarkRows() []string {
	rows := make([]string, len(letters[0]))

	for _, letter := range letters {
		for i, part := range letter {
			rows[i] += part
		}
	}

	return rows
}

type styledRun struct {
	style lipgloss.Style
	text  string
	plain bool
}

func renderWordmarkRow(t theme.Theme, row string, artWidth int) string {
	from, to, hasGradient := t.AccentGradient()

	var colors []color.Color
	if hasGradient {
		colors = lipgloss.Blend1D(artWidth, from, to)
	}

	runs := []styledRun{}

	for i, r := range []rune(row) {
		runs = appendRun(runs, wordmarkCell(t, r, colors, i))
	}

	var out strings.Builder

	for _, run := range runs {
		if run.plain {
			out.WriteString(run.text)

			continue
		}

		out.WriteString(run.style.Render(run.text))
	}

	return strings.TrimRight(out.String(), " ")
}

func wordmarkCell(t theme.Theme, r rune, colors []color.Color, index int) styledRun {
	switch {
	case r == ' ':
		return styledRun{text: " ", plain: true}
	case !t.Symbols.Unicode && r == solidBlock:
		return styledRun{style: t.Strong(t.Accent), text: "#"}
	case !t.Symbols.Unicode:
		return styledRun{text: " ", plain: true}
	case r != solidBlock:
		return styledRun{style: t.Subtle, text: string(r)}
	case colors != nil:
		return styledRun{style: t.Strong(lipgloss.NewStyle().Foreground(colors[index])), text: string(r)}
	default:
		return styledRun{style: t.Strong(t.Accent), text: string(r)}
	}
}

func appendRun(runs []styledRun, next styledRun) []styledRun {
	if len(runs) == 0 {
		return append(runs, next)
	}

	last := &runs[len(runs)-1]
	sameStyle := last.plain == next.plain && (last.plain || last.style.Render("x") == next.style.Render("x"))

	if sameStyle {
		last.text += next.text

		return runs
	}

	return append(runs, next)
}

func rail(t theme.Theme, width int) string {
	runs := []styledRun{}

	for i := range width {
		if i%railTieEvery == 1 {
			runs = appendRun(runs, styledRun{style: t.Muted, text: t.Symbols.RailTie})

			continue
		}

		runs = appendRun(runs, styledRun{style: t.Subtle, text: t.Symbols.RailTrack})
	}

	var out strings.Builder
	for _, run := range runs {
		out.WriteString(run.style.Render(run.text))
	}

	return out.String()
}
