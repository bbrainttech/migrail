package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr304.md
var mr304Docs string

type dropTableInUse struct{}

func (dropTableInUse) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR304",
		Slug:            "drop-table-in-use",
		Title:           "Dropping a table breaks code that still uses it",
		Category:        ir.CategoryCompatibility,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityWarning,
		Docs:            mr304Docs,
	}
}

func (dropTableInUse) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetDropStmt() == nil || node.Stmt.GetDropStmt().GetRemoveType() != pg_query.ObjectType_OBJECT_TABLE {
		return nil
	}

	findings := []ir.Finding{}

	for _, table := range existingTables(c, namesOf(node.Stmt.GetDropStmt().GetObjects())) {
		findings = append(findings, ir.Finding{
			Confidence: ir.ConfidencePossible,
			Title:      fmt.Sprintf("Dropping table %s breaks code that still uses it", quoted(table.String())),
			Why:        "During a rolling deploy, instances running the previous release still query this table. Those queries fail as soon as it's gone.",
			Fix: &ir.Fix{
				Summary:   "Drop the table in two deploys:",
				Framework: langSQL,
				Steps: []ir.FixStep{
					{Title: fmt.Sprintf("Remove every use of %s from the application and deploy", table.Name)},
					{Title: "Drop the table in a later deploy"},
				},
			},
		})
	}

	return findings
}
