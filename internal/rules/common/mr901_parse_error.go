package common

import (
	_ "embed"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr901.md
var mr901Docs string

func Rules() []analyze.Rule {
	return []analyze.Rule{parseError{}}
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

	return []ir.Finding{{
		Title:    "Could not parse SQL",
		Why:      stmt.ParseError.Message + ". Other statements in this file were still analyzed.",
		Location: ir.SourceLoc{Span: stmt.ParseError.Span},
	}}
}
