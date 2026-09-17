package markdown

import (
	"path/filepath"
	"strings"
	"testing"

	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/ui/term"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

func TestExplainGolden(t *testing.T) {
	t.Parallel()

	docs := ""

	for _, rule := range rules.All() {
		if rule.Meta().ID == "MR101" {
			docs = rule.Meta().Docs
		}
	}

	for _, profile := range golden.Profiles {
		for _, width := range golden.Widths {
			t.Run(golden.Name(profile, width), func(t *testing.T) {
				t.Parallel()

				renderer := Renderer{
					Theme:     theme.New(profile.Color, true, profile.Unicode),
					Width:     term.Capabilities{Width: width}.ContentWidth(),
					Highlight: pg.New().Highlight,
				}

				got := golden.Downsample(t, renderer.Render(docs), profile.Color)
				golden.Assert(t, filepath.Join("../../../testdata/golden/explain/MR101", golden.Name(profile, width)), got)
			})
		}
	}
}

func TestRenderElements(t *testing.T) {
	t.Parallel()

	source := strings.Join([]string{
		"# Title · here",
		"",
		"A paragraph with `code`, **bold** and a [link](https://example.com).",
		"",
		"- first item",
		"  continues here",
		"1. numbered",
		"",
		"| A | B |",
		"|---|---|",
		"| `x` | y |",
		"",
		"```sql",
		"SELECT 1;",
		"```",
	}, "\n")

	got := Renderer{Theme: theme.New(0, true, false), Width: 80}.Render(source)

	for _, want := range []string{
		"  Title - here",
		"  A paragraph with code, bold and a link (https://example.com).",
		"  -  first item continues here",
		"  1. numbered",
		"  A  B",
		"  x  y",
		"    SELECT 1;",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered output is missing %q:\n%s", want, got)
		}
	}
}
