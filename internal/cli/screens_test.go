package cli

import (
	"path/filepath"
	"testing"

	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/ui/term"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

func TestRulesGolden(t *testing.T) {
	t.Parallel()

	for _, profile := range golden.Profiles {
		t.Run(profile.Name, func(t *testing.T) {
			t.Parallel()

			got := golden.Downsample(t, renderRules(theme.New(profile.Color, true, profile.Unicode), rules.All()), profile.Color)
			golden.Assert(t, filepath.Join("../../testdata/golden/rules", profile.Name+".golden"), got)
		})
	}
}

func TestHelpGolden(t *testing.T) {
	t.Parallel()

	for _, profile := range golden.Profiles {
		for _, width := range golden.Widths {
			t.Run(golden.Name(profile, width), func(t *testing.T) {
				t.Parallel()

				root := newRootCommand(nil, nil)
				caps := term.Capabilities{Width: width}
				th := theme.New(profile.Color, true, profile.Unicode)

				got := golden.Downsample(t, renderHelp(root, th, caps.ContentWidth(), width, true), profile.Color)
				golden.Assert(t, filepath.Join("../../testdata/golden/help", golden.Name(profile, width)), got)

				check, _, err := root.Find([]string{"check"})
				if err != nil {
					t.Fatalf("find check: %v", err)
				}

				got = golden.Downsample(t, renderHelp(check, th, caps.ContentWidth(), width, false), profile.Color)
				golden.Assert(t, filepath.Join("../../testdata/golden/help-check", golden.Name(profile, width)), got)
			})
		}
	}
}
