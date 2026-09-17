package ir

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
}

type LockImpact struct {
	Mode    string
	Tables  []string
	Blocks  []string
	Rewrite bool
	Scan    bool
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
