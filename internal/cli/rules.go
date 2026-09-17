package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const tableGap = "  "

type ruleEntry struct {
	ID         string      `json:"id"`
	Slug       string      `json:"slug"`
	Title      string      `json:"title"`
	Severity   ir.Severity `json:"severity"`
	Category   ir.Category `json:"category"`
	Enabled    bool        `json:"enabled"`
	Dialects   []string    `json:"dialects"`
	MinVersion string      `json:"minVersion,omitempty"`
	MaxVersion string      `json:"maxVersion,omitempty"`
}

func newRulesCommand(ui *uiFlags) *cobra.Command {
	asJSON := false

	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List every rule with its severity and category",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if asJSON {
				return writeRulesJSON(cmd.OutOrStdout(), rules.All())
			}

			d := newDisplay(cmd.OutOrStdout(), ui.settings(ciEnabled(false)))
			if _, err := io.WriteString(d.out, renderRules(d.theme, rules.All())); err != nil {
				return internalError(fmt.Errorf("write rules: %w", err))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print rules as JSON")

	return cmd
}

func writeRulesJSON(w io.Writer, all []analyze.Rule) error {
	entries := make([]ruleEntry, 0, len(all))

	for _, rule := range all {
		meta := rule.Meta()
		entry := ruleEntry{
			ID:       meta.ID,
			Slug:     meta.Slug,
			Title:    meta.Title,
			Severity: ruleSeverity(meta),
			Category: meta.Category,
			Enabled:  meta.DefaultEnabled,
			Dialects: []string{},
		}

		for _, dialect := range meta.Dialects {
			entry.Dialects = append(entry.Dialects, string(dialect))
		}

		if meta.MinVersion != nil {
			entry.MinVersion = meta.MinVersion.String()
		}

		if meta.MaxVersion != nil {
			entry.MaxVersion = meta.MaxVersion.String()
		}

		entries = append(entries, entry)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(entries); err != nil {
		return internalError(fmt.Errorf("write rules: %w", err))
	}

	return nil
}

func ruleSeverity(meta ir.RuleMeta) ir.Severity {
	if !meta.DefaultEnabled {
		return ir.SeverityOff
	}

	return meta.DefaultSeverity
}

func versionRange(meta ir.RuleMeta) string {
	switch {
	case meta.MinVersion != nil && meta.MaxVersion != nil:
		return meta.MinVersion.String() + "-" + meta.MaxVersion.String()
	case meta.MinVersion != nil:
		return meta.MinVersion.String() + "+"
	case meta.MaxVersion != nil:
		return "up to " + meta.MaxVersion.String()
	default:
		return "all"
	}
}

func renderRules(t theme.Theme, all []analyze.Rule) string {
	header := []string{"ID", "Rule", "Severity", "Category", "Postgres"}
	rows := [][]string{}
	enabled := 0

	for _, rule := range all {
		meta := rule.Meta()
		if meta.DefaultEnabled {
			enabled++
		}

		rows = append(rows, []string{meta.ID, meta.Slug, string(ruleSeverity(meta)), string(meta.Category), versionRange(meta)})
	}

	widths := make([]int, len(header))
	for _, row := range append([][]string{header}, rows...) {
		for i, cell := range row {
			widths[i] = max(widths[i], len(cell))
		}
	}

	lines := []string{"  " + t.Strong(t.Muted).Render(joinCells(header, widths))}

	for i, row := range rows {
		lines = append(lines, "  "+ruleRow(t, all[i].Meta(), row, widths))
	}

	footer := fmt.Sprintf("%d rules %s %d enabled %s run ", len(all), t.Symbols.Dot, enabled, t.Symbols.Dot)
	lines = append(lines, "  "+t.Muted.Render(footer)+t.Fg.Render("migrail explain <ID>"))

	return strings.Join(lines, "\n") + "\n"
}

func joinCells(cells []string, widths []int) string {
	padded := make([]string, len(cells))
	for i, cell := range cells {
		padded[i] = components.PadRight(cell, widths[i])
	}

	return strings.TrimRight(strings.Join(padded, tableGap), " ")
}

func ruleRow(t theme.Theme, meta ir.RuleMeta, row []string, widths []int) string {
	if !meta.DefaultEnabled {
		return t.Muted.Render(joinCells(row, widths))
	}

	cells := []string{
		t.Strong(t.Fg).Render(components.PadRight(row[0], widths[0])),
		t.Fg.Render(components.PadRight(row[1], widths[1])),
		t.Severity(meta.DefaultSeverity).Render(components.PadRight(row[2], widths[2])),
		t.Fg.Render(components.PadRight(row[3], widths[3])),
		t.Fg.Render(row[4]),
	}

	return strings.Join(cells, tableGap)
}
