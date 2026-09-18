package cli

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/ui/term"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

var (
	colorModes = []string{string(term.ColorAuto), string(term.ColorAlways), string(term.ColorNever)}
	themeModes = []string{string(term.ThemeAuto), string(term.ThemeDark), string(term.ThemeLight), string(term.ThemeMono)}
)

type uiFlags struct {
	color        string
	theme        string
	ascii        bool
	noHyperlinks bool
	env          term.Env
}

func (f *uiFlags) register(cmd *cobra.Command) {
	flags := cmd.PersistentFlags()
	flags.StringVar(&f.color, "color", string(term.ColorAuto), "use color: auto, always or never")
	flags.StringVar(&f.theme, "theme", string(term.ThemeAuto), "color theme: auto, dark, light or mono")
	flags.BoolVar(&f.ascii, "ascii", false, "use ASCII symbols instead of Unicode")
	flags.BoolVar(&f.noHyperlinks, "no-hyperlinks", false, "don't print clickable file links")
}

func (f *uiFlags) validate() error {
	if !slices.Contains(colorModes, f.color) {
		return fmt.Errorf("invalid --color %q: use %s", f.color, strings.Join(colorModes, ", "))
	}

	if !slices.Contains(themeModes, f.theme) {
		return fmt.Errorf("invalid --theme %q: use %s", f.theme, strings.Join(themeModes, ", "))
	}

	return nil
}

func (f *uiFlags) settings(ci bool) term.Settings {
	return term.Settings{
		Color:        term.ColorMode(f.color),
		Theme:        term.ThemeMode(f.theme),
		ASCII:        f.ascii,
		NoHyperlinks: f.noHyperlinks,
		CI:           ci,
	}
}

type display struct {
	caps  term.Capabilities
	theme theme.Theme
	out   io.Writer
}

func newDisplay(w io.Writer, settings term.Settings) display {
	caps := term.Detect(w, os.Environ(), settings)

	return display{
		caps:  caps,
		theme: theme.New(caps.Profile, caps.Dark, caps.Unicode),
		out:   &colorprofile.Writer{Forward: w, Profile: caps.Profile},
	}
}

func fileLink(path string, line int) string {
	if path == stdinPath {
		return ""
	}

	absolute, err := filepath.Abs(filepath.FromSlash(path))
	if err != nil {
		return ""
	}

	link := url.URL{Scheme: "file", Path: filepath.ToSlash(absolute), Fragment: fmt.Sprintf("L%d", line)}

	return link.String()
}

func ciEnabled(flag bool) bool {
	return flag || term.IsCI(term.NewEnv(os.Environ()))
}
