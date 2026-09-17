package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr502.md
var mr502Docs string

type dropTable struct{}

func (dropTable) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR502",
		Slug:            "drop-table",
		Title:           "DROP TABLE deletes the table and its data",
		Category:        ir.CategoryData,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr502Docs,
	}
}

func (dropTable) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetDropStmt() == nil || node.Stmt.GetDropStmt().GetRemoveType() != pg_query.ObjectType_OBJECT_TABLE {
		return nil
	}

	findings := []ir.Finding{}

	for _, table := range existingTables(c, namesOf(node.Stmt.GetDropStmt().GetObjects())) {
		finding := ir.Finding{
			Title: fmt.Sprintf("Dropping table %s deletes its data", quoted(table.String())),
			Why: "The table and every row in it are gone once the migration commits. " +
				"Code that still queries the table also fails during a rolling deploy.",
			Lock: lockmodel.Impact(lockmodel.DropTable, table),
			Fix: &ir.Fix{
				Summary:   "Keep a way back until you're sure the data isn't needed:",
				Framework: langSQL,
				Steps: []ir.FixStep{
					{Title: "Take a backup of the table, or rename it instead of dropping it"},
					{Title: "Drop it in a later migration once nothing uses it"},
				},
			},
		}

		if span, ok := node.IdentifierSpan("TABLE", table.Name); ok {
			finding.Location.Span = span
		}

		findings = append(findings, finding)
	}

	return findings
}
