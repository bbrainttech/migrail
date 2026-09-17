package common

import (
	_ "embed"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr901.md
var mr901Docs string

func Rules() []analyze.Rule {
	return []analyze.Rule{editedAppliedMigration{}, parseError{}, ignoreWithoutReason{}, unusedIgnore{}}
}

type parseError struct{}

func (parseError) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR901",
		Slug:            "parse-error",
		Title:           "SQL could not be parsed",
		Category:        ir.CategoryDiagnostic,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr901Docs,
	}
}

func (parseError) Check(_ *analyze.Context, stmt *ir.Statement) []ir.Finding {
	if stmt.ParseError == nil {
		return nil
	}

	note := "Other statements in this file were still analyzed."
	if stmt.ParseError.CoversRest {
		note = "migrail can't read past this point, so the statements from here to the end of the file were not checked."
	}

	return []ir.Finding{{
		Title:    "Could not parse SQL",
		Why:      stmt.ParseError.Message + ". " + note,
		Label:    stmt.ParseError.Message,
		Note:     note,
		Location: ir.SourceLoc{Span: stmt.ParseError.Span},
	}}
}
