package github

import (
	"fmt"
	"io"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

const (
	commandError   = "error"
	commandWarning = "warning"
	commandNotice  = "notice"
)

var (
	messageEscaper  = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	propertyEscaper = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

type Input struct {
	Result analyze.Result
}

func Write(w io.Writer, in Input) error {
	var out strings.Builder

	counts := map[ir.Severity]int{}

	for _, finding := range in.Result.Findings {
		if finding.Suppressed != nil {
			continue
		}

		command := commandFor(finding.Severity)
		if command == "" {
			continue
		}

		counts[finding.Severity]++

		out.WriteString(annotation(command, finding))
	}

	for _, e := range in.Result.RuleErrors {
		fmt.Fprintf(&out, "::%s title=%s::%s\n", commandError, property("migrail rule error"),
			messageEscaper.Replace(fmt.Sprintf("%s failed on %s: %s", e.RuleID, e.MigrationID, e.Message)))
	}

	fmt.Fprintf(&out, "migrail: %s, %s, %s\n",
		count(counts[ir.SeverityError], "error", "errors"),
		count(counts[ir.SeverityWarning], "warning", "warnings"),
		count(counts[ir.SeverityNotice], "notice", "notices"),
	)

	if _, err := io.WriteString(w, out.String()); err != nil {
		return fmt.Errorf("write github annotations: %w", err)
	}

	return nil
}

func annotation(command string, finding ir.Finding) string {
	span := finding.Location.Span
	properties := []string{
		"file=" + property(finding.Location.Path),
		fmt.Sprintf("line=%d", span.Start.Line),
		fmt.Sprintf("endLine=%d", max(span.End.Line, span.Start.Line)),
	}

	if span.End.Line == span.Start.Line && span.End.Column > span.Start.Column {
		properties = append(properties, fmt.Sprintf("col=%d", span.Start.Column), fmt.Sprintf("endColumn=%d", span.End.Column))
	}

	properties = append(properties, "title="+property(finding.RuleID+" "+finding.Title))

	return fmt.Sprintf("::%s %s::%s\n", command, strings.Join(properties, ","), messageEscaper.Replace(message(finding)))
}

func message(finding ir.Finding) string {
	parts := []string{}

	if finding.Why != "" {
		parts = append(parts, finding.Why)
	}

	if finding.Fix != nil && finding.Fix.Summary != "" {
		parts = append(parts, "Fix: "+finding.Fix.Summary)

		for _, step := range finding.Fix.Steps {
			if text := step.PlainText(); text != "" {
				parts = append(parts, text)
			}
		}
	}

	parts = append(parts, "Run `migrail explain "+finding.RuleID+"` for details.")

	return strings.Join(parts, "\n\n")
}

func commandFor(severity ir.Severity) string {
	switch severity {
	case ir.SeverityError:
		return commandError
	case ir.SeverityWarning:
		return commandWarning
	case ir.SeverityNotice:
		return commandNotice
	case ir.SeverityOff:
		return ""
	default:
		return ""
	}
}

func property(value string) string {
	return propertyEscaper.Replace(value)
}

func count(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, components.Plural(n, one, many))
}
