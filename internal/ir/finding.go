package ir

import "strings"

type Severity string

const (
	SeverityOff     Severity = "off"
	SeverityNotice  Severity = "notice"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

func (s Severity) Rank() int {
	switch s {
	case SeverityError:
		return 3
	case SeverityWarning:
		return 2
	case SeverityNotice:
		return 1
	case SeverityOff:
		return 0
	default:
		return 0
	}
}

func (s Severity) AtLeast(threshold Severity) bool {
	return s.Rank() > 0 && s.Rank() >= threshold.Rank()
}

func (s Severity) FailsAt(failOn Severity) bool {
	return failOn.Rank() > 0 && s.AtLeast(failOn)
}

type Confidence string

const (
	ConfidenceDefinite Confidence = "definite"
	ConfidenceLikely   Confidence = "likely"
	ConfidencePossible Confidence = "possible"
)

type Category string

const (
	CategoryLocking       Category = "locking"
	CategoryRewrite       Category = "rewrite"
	CategoryCompatibility Category = "compatibility"
	CategoryTransaction   Category = "transaction"
	CategoryData          Category = "data"
	CategoryBestPractice  Category = "best-practice"
	CategoryDiagnostic    Category = "diagnostic"
)

type RuleMeta struct {
	ID              string
	Slug            string
	Title           string
	Category        Category
	Dialects        []Dialect
	DefaultSeverity Severity
	DefaultEnabled  bool
	MinVersion      *Version
	MaxVersion      *Version
	Docs            string
	Tags            []string
}

type Finding struct {
	RuleID      string
	Slug        string
	Severity    Severity
	Confidence  Confidence
	Title       string
	Why         string
	Label       string
	Note        string
	Lock        *LockImpact
	Evidence    []Evidence
	Fix         *Fix
	Location    SourceLoc
	MigrationID string
	Statement   *Statement
	Fingerprint string
	Suppressed  *Suppression
}

type Suppression struct {
	Reason string
	Source string
	Line   int
}

type LockImpact struct {
	Mode    string
	Tables  []string
	Blocks  []string
	Rewrite bool
	Scan    bool
}

func (l LockImpact) BlockedText() string {
	switch {
	case len(l.Blocks) == 0:
		return "doesn't block reads or writes"
	case len(l.Blocks) == 4:
		return "blocks reads and writes"
	default:
		return "blocks " + strings.Join(l.Blocks, ", ")
	}
}

type Evidence struct {
	Kind       string
	Message    string
	Location   *SourceLoc
	Confidence Confidence
}

type Fix struct {
	Summary   string
	Framework string
	Steps     []FixStep
}

type FixStep struct {
	Title string
	Lang  string
	Code  string
}

func (s FixStep) PlainText() string {
	switch {
	case s.Title != "" && s.Code != "":
		return s.Title + ":\n" + s.Code
	case s.Code != "":
		return s.Code
	default:
		return s.Title
	}
}
