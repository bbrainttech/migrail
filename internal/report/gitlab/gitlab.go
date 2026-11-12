package gitlab

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	severityCritical = "critical"
	severityMajor    = "major"
	severityInfo     = "info"
)

type Input struct {
	Result analyze.Result
}

type issue struct {
	Description string   `json:"description"`
	CheckName   string   `json:"check_name"`
	Fingerprint string   `json:"fingerprint"`
	Severity    string   `json:"severity"`
	Location    location `json:"location"`
}

type location struct {
	Path  string `json:"path"`
	Lines lines  `json:"lines"`
}

type lines struct {
	Begin int `json:"begin"`
}

func Write(w io.Writer, in Input) error {
	issues := []issue{}
	occurrences := map[string]int{}

	for _, finding := range in.Result.Findings {
		if finding.Suppressed != nil {
			continue
		}

		occurrences[finding.Fingerprint]++

		issues = append(issues, issue{
			Description: finding.RuleID + " " + finding.Title,
			CheckName:   finding.RuleID,
			Fingerprint: uniqueFingerprint(finding.Fingerprint, occurrences[finding.Fingerprint]),
			Severity:    severityOf(finding.Severity),
			Location: location{
				Path:  finding.Location.Path,
				Lines: lines{Begin: finding.Location.Span.Start.Line},
			},
		})
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(issues); err != nil {
		return fmt.Errorf("write GitLab report: %w", err)
	}

	return nil
}

func uniqueFingerprint(fingerprint string, occurrence int) string {
	if occurrence == 1 {
		return fingerprint
	}

	return fmt.Sprintf("%s-%d", fingerprint, occurrence)
}

func severityOf(severity ir.Severity) string {
	switch severity {
	case ir.SeverityError:
		return severityCritical
	case ir.SeverityWarning:
		return severityMajor
	case ir.SeverityNotice, ir.SeverityOff:
		return severityInfo
	default:
		return severityInfo
	}
}
