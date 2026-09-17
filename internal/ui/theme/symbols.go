package theme

type Symbols struct {
	Unicode     bool
	Error       string
	Warning     string
	Notice      string
	Success     string
	Suppressed  string
	FileMarker  string
	Branch      string
	LastBranch  string
	FirstBranch string
	Pipe        string
	Caret       string
	SpanMarker  string
	Rule        string
	Dot         string
	Ellipsis    string
	Bullet      string
	RailTrack   string
	RailTie     string
	Spinner     []string
}

func NewSymbols(unicode bool) Symbols {
	if unicode {
		return Symbols{
			Unicode:     true,
			Error:       "✖",
			Warning:     "▲",
			Notice:      "●",
			Success:     "✔",
			Suppressed:  "◌",
			FileMarker:  "▌",
			Branch:      "├─",
			LastBranch:  "╰─",
			FirstBranch: "╭─",
			Pipe:        "│",
			Caret:       "━",
			SpanMarker:  "┃",
			Rule:        "─",
			Dot:         "·",
			Ellipsis:    "…",
			Bullet:      "•",
			RailTrack:   "═",
			RailTie:     "╪",
			Spinner:     []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		}
	}

	return Symbols{
		Error:       "x",
		Warning:     "!",
		Notice:      "i",
		Success:     "ok",
		Suppressed:  "-",
		FileMarker:  ">",
		Branch:      "|-",
		LastBranch:  "`-",
		FirstBranch: ",-",
		Pipe:        "|",
		Caret:       "^",
		SpanMarker:  "|",
		Rule:        "-",
		Dot:         "-",
		Ellipsis:    "...",
		Bullet:      "-",
		RailTrack:   "=",
		RailTie:     "+",
		Spinner:     []string{"-", "\\", "|", "/"},
	}
}
