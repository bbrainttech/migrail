package sarif

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/suppress"
)

const (
	schemaURI      = "https://json.schemastore.org/sarif-2.1.0.json"
	sarifVersion   = "2.1.0"
	informationURI = "https://github.com/bbrainttech/migrail"
	srcRoot        = "%SRCROOT%"
	fingerprintKey = "migrail/v1"
	columnKind     = "unicodeCodePoints"
	suppressInline = "inSource"
	suppressConfig = "external"
	levelError     = "error"
	levelWarning   = "warning"
	levelNote      = "note"
	levelNone      = "none"
)

type Input struct {
	ToolVersion string
	Rules       []ir.RuleMeta
	Result      analyze.Result
}

type log struct {
	Schema  string `json:"$schema"`
	Version string `json:"version"`
	Runs    []run  `json:"runs"`
}

type run struct {
	Tool        tool         `json:"tool"`
	Invocations []invocation `json:"invocations"`
	ColumnKind  string       `json:"columnKind"`
	Results     []result     `json:"results"`
}

type tool struct {
	Driver driver `json:"driver"`
}

type driver struct {
	Name           string `json:"name"`
	Version        string `json:"version"`
	InformationURI string `json:"informationUri"`
	Rules          []rule `json:"rules"`
}

type rule struct {
	ID                   string        `json:"id"`
	Name                 string        `json:"name"`
	ShortDescription     text          `json:"shortDescription"`
	Help                 help          `json:"help"`
	DefaultConfiguration configuration `json:"defaultConfiguration"`
	Properties           ruleProps     `json:"properties"`
}

type text struct {
	Text string `json:"text"`
}

type help struct {
	Text     string `json:"text"`
	Markdown string `json:"markdown"`
}

type configuration struct {
	Level string `json:"level"`
}

type ruleProps struct {
	Tags []string `json:"tags"`
}

type invocation struct {
	ExecutionSuccessful bool           `json:"executionSuccessful"`
	Notifications       []notification `json:"toolExecutionNotifications"`
}

type notification struct {
	Level      string               `json:"level"`
	Message    text                 `json:"message"`
	Descriptor *reportingDescriptor `json:"associatedRule,omitempty"`
}

type reportingDescriptor struct {
	ID string `json:"id"`
}

type result struct {
	RuleID              string            `json:"ruleId"`
	RuleIndex           int               `json:"ruleIndex"`
	Level               string            `json:"level"`
	Message             text              `json:"message"`
	Locations           []location        `json:"locations"`
	PartialFingerprints map[string]string `json:"partialFingerprints"`
	Suppressions        []suppression     `json:"suppressions,omitempty"`
	Properties          resultProps       `json:"properties"`
}

type location struct {
	PhysicalLocation physicalLocation `json:"physicalLocation"`
}

type physicalLocation struct {
	ArtifactLocation artifactLocation `json:"artifactLocation"`
	Region           region           `json:"region"`
}

type artifactLocation struct {
	URI       string `json:"uri"`
	URIBaseID string `json:"uriBaseId"`
}

type region struct {
	StartLine   int `json:"startLine"`
	StartColumn int `json:"startColumn"`
	EndLine     int `json:"endLine"`
	EndColumn   int `json:"endColumn"`
}

type suppression struct {
	Kind          string `json:"kind"`
	Justification string `json:"justification,omitempty"`
}

type resultProps struct {
	Confidence ir.Confidence `json:"confidence"`
	Slug       string        `json:"slug"`
}

func Write(w io.Writer, in Input) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(build(in)); err != nil {
		return fmt.Errorf("write sarif report: %w", err)
	}

	return nil
}

func build(in Input) log {
	rules := make([]rule, 0, len(in.Rules))
	indexes := make(map[string]int, len(in.Rules))

	for i, meta := range in.Rules {
		rules = append(rules, buildRule(meta))
		indexes[meta.ID] = i
	}

	results := make([]result, 0, len(in.Result.Findings))

	for _, finding := range in.Result.Findings {
		index, ok := indexes[finding.RuleID]
		if !ok {
			continue
		}

		results = append(results, buildResult(finding, index))
	}

	return log{
		Schema:  schemaURI,
		Version: sarifVersion,
		Runs: []run{{
			Tool:        tool{Driver: driver{Name: "migrail", Version: in.ToolVersion, InformationURI: informationURI, Rules: rules}},
			Invocations: []invocation{buildInvocation(in.Result.RuleErrors)},
			ColumnKind:  columnKind,
			Results:     results,
		}},
	}
}

func buildRule(meta ir.RuleMeta) rule {
	return rule{
		ID:                   meta.ID,
		Name:                 meta.Slug,
		ShortDescription:     text{Text: meta.Title},
		Help:                 help{Text: meta.Title, Markdown: meta.Docs},
		DefaultConfiguration: configuration{Level: level(meta.DefaultSeverity)},
		Properties:           ruleProps{Tags: []string{string(meta.Category)}},
	}
}

func buildResult(finding ir.Finding, index int) result {
	out := result{
		RuleID:              finding.RuleID,
		RuleIndex:           index,
		Level:               level(finding.Severity),
		Message:             text{Text: message(finding)},
		Locations:           []location{{PhysicalLocation: physical(finding.Location)}},
		PartialFingerprints: map[string]string{fingerprintKey: finding.Fingerprint},
		Properties:          resultProps{Confidence: finding.Confidence, Slug: finding.Slug},
	}

	if finding.Suppressed != nil {
		kind := suppressConfig
		if finding.Suppressed.Source == suppress.SourceInline {
			kind = suppressInline
		}

		out.Suppressions = []suppression{{Kind: kind, Justification: finding.Suppressed.Reason}}
	}

	return out
}

func message(finding ir.Finding) string {
	parts := []string{finding.Title + "."}

	if finding.Why != "" {
		parts = append(parts, finding.Why)
	}

	if finding.Fix != nil && finding.Fix.Summary != "" {
		parts = append(parts, "Fix: "+finding.Fix.Summary)

		for _, step := range finding.Fix.Steps {
			parts = append(parts, step.PlainText())
		}
	}

	return strings.Join(parts, "\n\n")
}

func physical(loc ir.SourceLoc) physicalLocation {
	span := loc.Span
	end := span.End

	if end.Line < span.Start.Line || (end.Line == span.Start.Line && end.Column <= span.Start.Column) {
		end = ir.Position{Line: span.Start.Line, Column: span.Start.Column + 1}
	}

	return physicalLocation{
		ArtifactLocation: artifactLocation{URI: loc.Path, URIBaseID: srcRoot},
		Region: region{
			StartLine:   span.Start.Line,
			StartColumn: span.Start.Column,
			EndLine:     end.Line,
			EndColumn:   end.Column,
		},
	}
}

func buildInvocation(errors []analyze.RuleError) invocation {
	notifications := make([]notification, 0, len(errors))

	for _, e := range errors {
		notifications = append(notifications, notification{
			Level:      levelError,
			Message:    text{Text: fmt.Sprintf("%s failed on %s: %s", e.RuleID, e.MigrationID, e.Message)},
			Descriptor: &reportingDescriptor{ID: e.RuleID},
		})
	}

	return invocation{ExecutionSuccessful: len(errors) == 0, Notifications: notifications}
}

func level(severity ir.Severity) string {
	switch severity {
	case ir.SeverityError:
		return levelError
	case ir.SeverityWarning:
		return levelWarning
	case ir.SeverityNotice:
		return levelNote
	case ir.SeverityOff:
		return levelNone
	default:
		return levelNone
	}
}
