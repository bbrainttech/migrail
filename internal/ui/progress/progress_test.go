package progress

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

func TestProgressLinesGolden(t *testing.T) {
	t.Parallel()

	for _, profile := range golden.Profiles {
		t.Run(profile.Name, func(t *testing.T) {
			t.Parallel()

			th := theme.New(profile.Color, true, profile.Unicode)
			lines := []string{
				ActiveLine(th, 2, "Extracting SQL", "django · 2 migrations  (python manage.py sqlmigrate)"),
				DoneLine(th, "Discovered", "3 migrations · goose", 4*time.Millisecond),
				DoneLine(th, "Extracted SQL", "static", 2*time.Millisecond),
				DoneLine(th, "Analyzed", "14 statements · 6 rules", 1250*time.Millisecond),
			}

			got := golden.Downsample(t, strings.Join(lines, "\n")+"\n", profile.Color)
			golden.Assert(t, filepath.Join("../../../testdata/golden/progress", profile.Name+".golden"), got)
		})
	}
}

func TestVerbosePhasesWithoutAnimation(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	p := New(&out, &out, theme.New(0, true, false), Options{Verbose: true})
	p.Start("Discovering", "1 file").Done("Discovered", "1 migration")
	p.Start("Analyzing", "6 rules").Abort()

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "ok Discovered") {
		t.Errorf("verbose output = %q", out.String())
	}
}

func TestSpinnerAppearsOnlyForSlowPhases(t *testing.T) {
	t.Parallel()

	var fast bytes.Buffer

	New(&fast, &fast, theme.New(0, true, true), Options{Animate: true}).Start("Analyzing", "").Done("Analyzed", "")

	if fast.Len() != 0 {
		t.Errorf("fast phase drew a spinner: %q", fast.String())
	}

	var slow bytes.Buffer

	phase := New(&slow, &slow, theme.New(0, true, true), Options{Animate: true}).Start("Analyzing", "6 rules")

	time.Sleep(appearAfter + 2*frameInterval)
	phase.Done("Analyzed", "")

	if !strings.Contains(slow.String(), "Analyzing") || !strings.HasSuffix(slow.String(), clearLine) {
		t.Errorf("slow phase output = %q", slow.String())
	}
}
