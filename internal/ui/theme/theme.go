package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/bbrainttech/migrail/internal/ir"
)

type token struct {
	dark  string
	light string
	basic color.Color
}

var (
	tokenFg      = token{dark: "#E6E6EA", light: "#1F2328"}
	tokenMuted   = token{dark: "#8B8D98", light: "#6E7781", basic: lipgloss.BrightBlack}
	tokenSubtle  = token{dark: "#3A3C45", light: "#D0D7DE", basic: lipgloss.BrightBlack}
	tokenAccent  = token{dark: "#7C8CFF", light: "#4F5BD5", basic: lipgloss.Blue}
	tokenError   = token{dark: "#FF6B81", light: "#CF222E", basic: lipgloss.Red}
	tokenWarning = token{dark: "#F5B95C", light: "#9A6700", basic: lipgloss.Yellow}
	tokenNotice  = token{dark: "#6CC4FF", light: "#0969DA", basic: lipgloss.Cyan}
	tokenSuccess = token{dark: "#5FD69B", light: "#1A7F37", basic: lipgloss.Green}
	tokenKeyword = token{dark: "#C792EA", light: "#8250DF", basic: lipgloss.Magenta}
	tokenString  = token{dark: "#A5D6A7", light: "#0A3069", basic: lipgloss.Green}
)

type Theme struct {
	Profile colorprofile.Profile
	Dark    bool
	Symbols Symbols

	Fg      lipgloss.Style
	Bold    lipgloss.Style
	Muted   lipgloss.Style
	Subtle  lipgloss.Style
	Accent  lipgloss.Style
	Error   lipgloss.Style
	Warning lipgloss.Style
	Notice  lipgloss.Style
	Success lipgloss.Style
	Keyword lipgloss.Style
	String  lipgloss.Style
	Comment lipgloss.Style
}

func New(profile colorprofile.Profile, dark, unicode bool) Theme {
	t := Theme{Profile: profile, Dark: dark, Symbols: NewSymbols(unicode)}

	t.Fg = t.style(tokenFg)
	t.Bold = t.style(tokenFg).Bold(t.decorated())
	t.Muted = t.style(tokenMuted)
	t.Subtle = t.style(tokenSubtle)
	t.Accent = t.style(tokenAccent)
	t.Error = t.style(tokenError)
	t.Warning = t.style(tokenWarning)
	t.Notice = t.style(tokenNotice)
	t.Success = t.style(tokenSuccess)
	t.Keyword = t.style(tokenKeyword)
	t.String = t.style(tokenString)
	t.Comment = t.style(tokenMuted)

	return t
}

func (t Theme) decorated() bool {
	return t.Profile >= colorprofile.ASCII
}

func (t Theme) HasColor() bool {
	return t.Profile >= colorprofile.ANSI
}

func (t Theme) style(tok token) lipgloss.Style {
	style := lipgloss.NewStyle()

	switch {
	case t.Profile >= colorprofile.ANSI256:
		hex := tok.dark
		if !t.Dark {
			hex = tok.light
		}

		return style.Foreground(lipgloss.Color(hex))
	case t.Profile == colorprofile.ANSI && tok.basic != nil:
		return style.Foreground(tok.basic)
	default:
		return style
	}
}

func (t Theme) AccentGradient() (from, to color.Color, ok bool) {
	if t.Profile < colorprofile.ANSI256 {
		return nil, nil, false
	}

	if t.Dark {
		return lipgloss.Color(tokenAccent.dark), lipgloss.Color(tokenNotice.dark), true
	}

	return lipgloss.Color(tokenAccent.light), lipgloss.Color(tokenNotice.light), true
}

func (t Theme) Severity(severity ir.Severity) lipgloss.Style {
	switch severity {
	case ir.SeverityError:
		return t.Error
	case ir.SeverityWarning:
		return t.Warning
	case ir.SeverityNotice:
		return t.Notice
	case ir.SeverityOff:
		return t.Muted
	default:
		return t.Fg
	}
}

func (t Theme) SeveritySymbol(severity ir.Severity) string {
	switch severity {
	case ir.SeverityError:
		return t.Symbols.Error
	case ir.SeverityWarning:
		return t.Symbols.Warning
	case ir.SeverityNotice:
		return t.Symbols.Notice
	case ir.SeverityOff:
		return t.Symbols.Suppressed
	default:
		return t.Symbols.Notice
	}
}

func (t Theme) Strong(style lipgloss.Style) lipgloss.Style {
	return style.Bold(t.decorated())
}

func (t Theme) Dim(style lipgloss.Style) lipgloss.Style {
	return style.Faint(t.decorated() && !t.HasColor())
}
