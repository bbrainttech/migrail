package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/ui/markdown"
)

func newExplainCommand(ui *uiFlags) *cobra.Command {
	noPager := false

	cmd := &cobra.Command{
		Use:   "explain <rule>",
		Short: "Explain what a rule detects and how to fix it",
		Long:  "Explain what a rule detects, why it's dangerous and how to fix it. Pass a rule ID such as MR101 or a slug such as create-index-non-concurrent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rule, err := findRule(rules.All(), args[0])
			if err != nil {
				return err
			}

			d := newDisplay(cmd.OutOrStdout(), ui.settings(ciEnabled(false)))
			renderer := markdown.Renderer{
				Theme:      d.theme,
				Width:      d.caps.ContentWidth(),
				Highlight:  pg.New().Highlight,
				Hyperlinks: d.caps.Hyperlinks,
			}

			var rendered bytes.Buffer

			converted := &colorprofile.Writer{Forward: &rendered, Profile: d.caps.Profile}
			if _, err := io.WriteString(converted, "\n"+renderer.Render(rule.Meta().Docs)); err != nil {
				return internalError(fmt.Errorf("render rule docs: %w", err))
			}

			usePager := d.caps.TTY && !noPager && d.caps.Height > 0 && strings.Count(rendered.String(), "\n") > d.caps.Height
			if usePager && runPager(cmd.Context(), rendered.String(), cmd.OutOrStdout()) == nil {
				return nil
			}

			if _, err := cmd.OutOrStdout().Write(rendered.Bytes()); err != nil {
				return internalError(fmt.Errorf("write rule docs: %w", err))
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&noPager, "no-pager", false, "print directly instead of opening a pager")

	return cmd
}

func findRule(all []analyze.Rule, name string) (analyze.Rule, error) {
	candidates := []string{}

	for _, rule := range all {
		meta := rule.Meta()
		if strings.EqualFold(meta.ID, name) || meta.Slug == strings.ToLower(name) {
			return rule, nil
		}

		candidates = append(candidates, meta.ID, meta.Slug)
	}

	if suggestion := closest(strings.ToLower(name), candidates); suggestion != "" {
		return nil, fmt.Errorf("unknown rule %q: did you mean %s? Run migrail rules to list every rule", name, suggestion)
	}

	return nil, fmt.Errorf("unknown rule %q: run migrail rules to list every rule", name)
}

func closest(name string, candidates []string) string {
	best, bestDistance := "", len(name)/2+1

	for _, candidate := range candidates {
		if distance := levenshtein(name, strings.ToLower(candidate)); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}

	return best
}

func levenshtein(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}

		previous = current
	}

	return previous[len(b)]
}

func runPager(ctx context.Context, text string, out io.Writer) error {
	command := strings.Fields(os.Getenv("PAGER"))
	if len(command) == 0 {
		command = []string{"less", "-FRX"}
	}

	pager := exec.CommandContext(ctx, command[0], command[1:]...) //nolint:gosec // the pager comes from the user's own PAGER variable
	pager.Stdin = strings.NewReader(text)
	pager.Stdout = out
	pager.Stderr = os.Stderr

	if err := pager.Run(); err != nil {
		return fmt.Errorf("run pager %s: %w", command[0], err)
	}

	return nil
}
