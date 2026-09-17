package golden

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

type Profile struct {
	Name    string
	Color   colorprofile.Profile
	Unicode bool
}

var (
	Profiles = []Profile{
		{Name: "truecolor", Color: colorprofile.TrueColor, Unicode: true},
		{Name: "ansi16", Color: colorprofile.ANSI, Unicode: true},
		{Name: "nocolor", Color: colorprofile.ASCII, Unicode: true},
		{Name: "ascii", Color: colorprofile.NoTTY, Unicode: false},
	}
	Widths = []int{60, 80, 120}
)

func Name(profile Profile, width int) string {
	return fmt.Sprintf("%s-%d.golden", profile.Name, width)
}

func Downsample(t *testing.T, text string, profile colorprofile.Profile) string {
	t.Helper()

	var out bytes.Buffer

	writer := &colorprofile.Writer{Forward: &out, Profile: profile}
	if _, err := io.WriteString(writer, text); err != nil {
		t.Fatalf("downsample: %v", err)
	}

	return out.String()
}

func Assert(t *testing.T, path, got string) {
	t.Helper()

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}

		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run make golden-update): %v", path, err)
	}

	if got != string(want) {
		t.Errorf("output differs from %s (run make golden-update and review the diff)\ngot:\n%s", path, got)
	}
}
