package term

import (
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	xterm "golang.org/x/term"
)

const (
	MaxContentWidth = 100
	MinPrettyWidth  = 60
	defaultWidth    = 80
)

type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

type ThemeMode string

const (
	ThemeAuto  ThemeMode = "auto"
	ThemeDark  ThemeMode = "dark"
	ThemeLight ThemeMode = "light"
	ThemeMono  ThemeMode = "mono"
)

type Settings struct {
	Color        ColorMode
	Theme        ThemeMode
	ASCII        bool
	NoHyperlinks bool
	CI           bool
}

type Capabilities struct {
	TTY        bool
	Profile    colorprofile.Profile
	Unicode    bool
	Dark       bool
	Hyperlinks bool
	Width      int
	Height     int
}

func (c Capabilities) ContentWidth() int {
	return min(c.Width, MaxContentWidth)
}

func (c Capabilities) Compact() bool {
	return c.Width < MinPrettyWidth
}

type Env map[string]string

func NewEnv(environ []string) Env {
	env := make(Env, len(environ))

	for _, entry := range environ {
		if key, value, ok := strings.Cut(entry, "="); ok {
			env[key] = value
		}
	}

	return env
}

func Detect(out io.Writer, environ []string, settings Settings) Capabilities {
	env := NewEnv(environ)
	file, isFile := out.(*os.File)
	tty := isFile && xterm.IsTerminal(int(file.Fd()))

	caps := Capabilities{
		TTY:     tty,
		Unicode: !settings.ASCII && supportsUnicode(env),
		Width:   defaultWidth,
		Height:  0,
	}

	if tty {
		if width, height, err := xterm.GetSize(int(file.Fd())); err == nil && width > 0 {
			caps.Width, caps.Height = width, height
		}
	} else if columns, err := strconv.Atoi(env["COLUMNS"]); err == nil && columns > 0 {
		caps.Width = columns
	}

	caps.Profile = detectProfile(out, environ, env, settings)
	caps.Hyperlinks = tty && !settings.CI && !settings.NoHyperlinks && supportsHyperlinks(env)
	caps.Dark = detectDark(file, tty, env, settings, caps.Profile)

	return caps
}

func IsCI(env Env) bool {
	value := strings.ToLower(env["CI"])

	return value == "true" || value == "1"
}

func detectProfile(out io.Writer, environ []string, env Env, settings Settings) colorprofile.Profile {
	forced := env["FORCE_COLOR"] != "" && env["FORCE_COLOR"] != "0" || env["CLICOLOR_FORCE"] != "" && env["CLICOLOR_FORCE"] != "0"

	var profile colorprofile.Profile

	switch {
	case settings.Color == ColorNever:
		profile = colorprofile.NoTTY
	case settings.Color == ColorAlways || forced:
		profile = max(colorprofile.Env(environ), colorprofile.ANSI)
	case settings.CI:
		profile = colorprofile.NoTTY
	default:
		profile = colorprofile.Detect(out, environ)
	}

	if settings.Theme == ThemeMono && profile > colorprofile.ASCII {
		profile = colorprofile.ASCII
	}

	return profile
}

func supportsUnicode(env Env) bool {
	if runtime.GOOS == "windows" {
		return env["WT_SESSION"] != "" || env["TERM_PROGRAM"] != "" || env["TERM"] != ""
	}

	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.ToLower(env[key])
		if value == "" {
			continue
		}

		return strings.Contains(value, "utf-8") || strings.Contains(value, "utf8")
	}

	return false
}

func supportsHyperlinks(env Env) bool {
	switch env["TERM_PROGRAM"] {
	case "iTerm.app", "WezTerm", "vscode", "ghostty", "Hyper", "rio":
		return true
	}

	if env["WT_SESSION"] != "" || env["KITTY_WINDOW_ID"] != "" || env["KONSOLE_VERSION"] != "" {
		return true
	}

	vte, err := strconv.Atoi(env["VTE_VERSION"])

	return err == nil && vte >= 5000
}

func detectDark(file *os.File, tty bool, env Env, settings Settings, profile colorprofile.Profile) bool {
	switch settings.Theme {
	case ThemeDark, ThemeMono:
		return true
	case ThemeLight:
		return false
	case ThemeAuto:
	}

	if profile <= colorprofile.ASCII {
		return true
	}

	if dark, ok := darkFromColorFGBG(env["COLORFGBG"]); ok {
		return dark
	}

	if !tty || !xterm.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}

	return lipgloss.HasDarkBackground(os.Stdin, file)
}

func darkFromColorFGBG(value string) (dark, ok bool) {
	parts := strings.Split(value, ";")

	background, err := strconv.Atoi(parts[len(parts)-1])
	if value == "" || err != nil {
		return false, false
	}

	return background < 7 || background == 8, true
}
