package components

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

func TestBannerGolden(t *testing.T) {
	t.Parallel()

	for _, profile := range golden.Profiles {
		for _, width := range append([]int{50}, golden.Widths...) {
			t.Run(golden.Name(profile, width), func(t *testing.T) {
				t.Parallel()

				rendered := Banner(theme.New(profile.Color, true, profile.Unicode), width)
				got := golden.Downsample(t, rendered, profile.Color)
				golden.Assert(t, filepath.Join("../../../testdata/golden/banner", golden.Name(profile, width)), got)
			})
		}
	}
}

func TestWordmarkRowsHaveEqualWidth(t *testing.T) {
	t.Parallel()

	rows := wordmarkRows()
	want := len([]rune(rows[0]))

	for i, row := range rows {
		if got := len([]rune(row)); got != want {
			t.Errorf("row %d is %d wide, want %d", i, got, want)
		}
	}

	if want+len(bannerIndent) > bannerMinimum {
		t.Errorf("wordmark is %d wide and doesn't fit the %d column minimum", want, bannerMinimum)
	}

	if strings.TrimSpace(rows[0]) == "" {
		t.Error("first wordmark row is empty")
	}
}
