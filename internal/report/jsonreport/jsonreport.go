package jsonreport

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

const SchemaVersion = 1

type Input struct {
	ToolVersion string
	Dialect     ir.Dialect
	DBVersion   ir.Version
	Migrations  []*ir.Migration
	Result      analyze.Result
	Base        string
	ChangedOnly bool
}

type report struct {
	Schema     int         `json:"schema"`
	Tool       tool        `json:"tool"`
	Database   database    `json:"database"`
	Scope      scope       `json:"scope"`
	Summary    summary     `json:"summary"`
	Migrations []migration `json:"migrations"`
	Findings   []finding   `json:"findings"`
	RuleErrors []ruleError `json:"ruleErrors"`
}

type tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type database struct {
	Dialect ir.Dialect `json:"dialect"`
	Version string     `json:"version"`
}

type scope struct {
	Base        string `json:"base,omitempty"`
	ChangedOnly bool   `json:"changedOnly"`
}

type summary struct {
	Migrations int `json:"migrations"`
	Statements int `json:"statements"`
	Errors     int `json:"errors"`
	Warnings   int `json:"warnings"`
	Notices    int `json:"notices"`
}

type migration struct {
	ID          string         `json:"id"`
	Path        string         `json:"path"`
	ChangeState ir.ChangeState `json:"changeState"`
	Framework   string         `json:"framework"`
	TxMode      ir.TxMode      `json:"txMode"`
	Statements  int            `json:"statements"`
}

type finding struct {
	RuleID      string        `json:"ruleId"`
	Slug        string        `json:"slug"`
	Severity    ir.Severity   `json:"severity"`
	Confidence  ir.Confidence `json:"confidence"`
	Title       string        `json:"title"`
	Why         string        `json:"why"`
	Location    location      `json:"location"`
	Statement   statement     `json:"statement"`
	Lock        *lock         `json:"lock,omitempty"`
	Fix         *fix          `json:"fix,omitempty"`
	MigrationID string        `json:"migrationId"`
	Fingerprint string        `json:"fingerprint"`
}

type location struct {
	Path  string   `json:"path"`
	Start position `json:"start"`
	End   position `json:"end"`
}

type position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
	Offset int `json:"offset"`
}

type statement struct {
	Index         int    `json:"index"`
	SQL           string `json:"sql"`
	InTransaction bool   `json:"inTransaction"`
}

type lock struct {
	Mode    string   `json:"mode"`
	Tables  []string `json:"tables"`
	Blocks  []string `json:"blocks"`
	Rewrite bool     `json:"rewrite"`
	Scan    bool     `json:"scan"`
}

type fix struct {
	Summary   string    `json:"summary"`
	Framework string    `json:"framework"`
	Steps     []fixStep `json:"steps"`
}

type fixStep struct {
	Title string `json:"title,omitempty"`
	Lang  string `json:"lang,omitempty"`
	Code  string `json:"code,omitempty"`
}

type ruleError struct {
	RuleID      string `json:"ruleId"`
	MigrationID string `json:"migrationId"`
	Statement   int    `json:"statement"`
	Message     string `json:"message"`
}

func Write(w io.Writer, in Input) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(build(in)); err != nil {
		return fmt.Errorf("write json report: %w", err)
	}

	return nil
}

func build(in Input) report {
	out := report{
		Schema:     SchemaVersion,
		Tool:       tool{Name: "migrail", Version: in.ToolVersion},
		Database:   database{Dialect: in.Dialect, Version: in.DBVersion.String()},
		Scope:      scope{Base: in.Base, ChangedOnly: in.ChangedOnly},
		Migrations: make([]migration, 0, len(in.Migrations)),
		Findings:   make([]finding, 0, len(in.Result.Findings)),
		RuleErrors: make([]ruleError, 0, len(in.Result.RuleErrors)),
		Summary:    summary{Migrations: len(in.Migrations), Statements: in.Result.Statements},
	}

	for _, m := range in.Migrations {
		out.Migrations = append(out.Migrations, migration{
			ID:          m.ID,
			Path:        m.SourcePath,
			ChangeState: m.ChangeState,
			Framework:   m.Framework,
			TxMode:      m.TxMode,
			Statements:  len(m.Statements),
		})
	}

	for _, f := range in.Result.Findings {
		out.Findings = append(out.Findings, buildFinding(f))
		countSeverity(&out.Summary, f.Severity)
	}

	for _, e := range in.Result.RuleErrors {
		out.RuleErrors = append(out.RuleErrors, ruleError(e))
	}

	return out
}

func countSeverity(s *summary, severity ir.Severity) {
	switch severity {
	case ir.SeverityError:
		s.Errors++
	case ir.SeverityWarning:
		s.Warnings++
	case ir.SeverityNotice:
		s.Notices++
	case ir.SeverityOff:
	}
}

func buildFinding(f ir.Finding) finding {
	out := finding{
		RuleID:     f.RuleID,
		Slug:       f.Slug,
		Severity:   f.Severity,
		Confidence: f.Confidence,
		Title:      f.Title,
		Why:        f.Why,
		Location: location{
			Path:  f.Location.Path,
			Start: buildPosition(f.Location.Span.Start),
			End:   buildPosition(f.Location.Span.End),
		},
		MigrationID: f.MigrationID,
		Fingerprint: f.Fingerprint,
	}

	if f.Statement != nil {
		out.Statement = statement{Index: f.Statement.Index, SQL: f.Statement.SQL, InTransaction: f.Statement.InTx}
	}

	if f.Lock != nil {
		out.Lock = &lock{Mode: f.Lock.Mode, Tables: f.Lock.Tables, Blocks: f.Lock.Blocks, Rewrite: f.Lock.Rewrite, Scan: f.Lock.Scan}
	}

	if f.Fix != nil {
		out.Fix = &fix{Summary: f.Fix.Summary, Framework: f.Fix.Framework, Steps: make([]fixStep, 0, len(f.Fix.Steps))}
		for _, step := range f.Fix.Steps {
			out.Fix.Steps = append(out.Fix.Steps, fixStep(step))
		}
	}

	return out
}

func buildPosition(p ir.Position) position {
	return position{Line: p.Line, Column: p.Column, Offset: p.Offset}
}
