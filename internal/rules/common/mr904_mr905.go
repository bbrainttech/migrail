package common

import (
	_ "embed"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

var (
	//go:embed mr904.md
	mr904Docs string

	//go:embed mr905.md
	mr905Docs string
)

type ignoreWithoutReason struct{}

func (ignoreWithoutReason) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR904",
		Slug:            "ignore-without-reason",
		Title:           "An ignore comment has no reason",
		Category:        ir.CategoryDiagnostic,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr904Docs,
	}
}

func (ignoreWithoutReason) Check(*analyze.Context, *ir.Statement) []ir.Finding {
	return nil
}

type unusedIgnore struct{}

func (unusedIgnore) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR905",
		Slug:            "unused-ignore",
		Title:           "An ignore comment doesn't match any finding",
		Category:        ir.CategoryDiagnostic,
		DefaultSeverity: ir.SeverityNotice,
		DefaultEnabled:  true,
		Docs:            mr905Docs,
	}
}

func (unusedIgnore) Check(*analyze.Context, *ir.Statement) []ir.Finding {
	return nil
}
