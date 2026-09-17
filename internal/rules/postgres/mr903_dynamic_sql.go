package postgres

import (
	_ "embed"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr903.md
var mr903Docs string

type dynamicSQL struct{}

func (dynamicSQL) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR903",
		Slug:            "dynamic-sql",
		Title:           "SQL inside a DO block can't be checked",
		Category:        ir.CategoryDiagnostic,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityNotice,
		DefaultEnabled:  true,
		Docs:            mr903Docs,
	}
}

func (dynamicSQL) Check(_ *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetDoStmt() == nil {
		return nil
	}

	return []ir.Finding{{
		Title: "migrail can't check the SQL inside this DO block",
		Why:   "Statements inside DO blocks and EXECUTE run as PL/pgSQL, so migrail doesn't see which tables they lock or change.",
		Note:  "Review the statements in the block by hand, or move plain DDL out of the block so migrail can check it.",
	}}
}
