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

//go:embed mr302.md
var mr302Docs string

type renameColumn struct{}

func (renameColumn) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR302",
		Slug:            "rename-column",
		Title:           "Renaming a column breaks code that still uses the old name",
		Category:        ir.CategoryCompatibility,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr302Docs,
	}
}

func (renameColumn) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetRenameStmt() == nil {
		return nil
	}

	rename := node.Stmt.GetRenameStmt()
	if rename.GetRenameType() != pg_query.ObjectType_OBJECT_COLUMN || rename.GetRelation() == nil {
		return nil
	}

	table := pg.RelationRef(rename.GetRelation())
	if c.IsNewTable(table) {
		return nil
	}

	column := ir.ColumnRef{Table: table, Name: rename.GetSubname()}
	newName := rename.GetNewname()

	finding := ir.Finding{
		Confidence: ir.ConfidenceLikely,
		Title:      fmt.Sprintf("Renaming column %s breaks running code", quoted(column.String())),
		Why: fmt.Sprintf(
			"During a rolling deploy, instances running the previous release still query %s. Those queries fail as soon as this migration runs.",
			quoted(column.Name),
		),
		Lock: lockmodel.Impact(lockmodel.RenameColumn, table),
		Fix: &ir.Fix{
			Summary:   "Rename in three deploys with expand and contract:",
			Framework: langSQL,
			Steps: []ir.FixStep{
				{Title: fmt.Sprintf("Add %s, write to both columns and backfill it", newName)},
				{Title: fmt.Sprintf("Read from %s and stop writing %s", newName, column.Name)},
				{Title: "Drop " + column.Name},
			},
		},
	}

	if span, ok := node.IdentifierSpan("RENAME", column.Name); ok {
		finding.Location.Span = span
	}

	return []ir.Finding{finding}
}
