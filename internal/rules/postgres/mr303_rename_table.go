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

//go:embed mr303.md
var mr303Docs string

type renameTable struct{}

func (renameTable) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR303",
		Slug:            "rename-table",
		Title:           "Renaming a table breaks code that still uses the old name",
		Category:        ir.CategoryCompatibility,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr303Docs,
	}
}

func (renameTable) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetRenameStmt() == nil {
		return nil
	}

	rename := node.Stmt.GetRenameStmt()
	if rename.GetRenameType() != pg_query.ObjectType_OBJECT_TABLE || rename.GetRelation() == nil {
		return nil
	}

	table := pg.RelationRef(rename.GetRelation())
	if c.IsNewTable(table) {
		return nil
	}

	renamed := ir.ObjectRef{Schema: table.Schema, Name: rename.GetNewname()}
	finding := ir.Finding{
		Confidence: ir.ConfidenceLikely,
		Title:      fmt.Sprintf("Renaming table %s breaks running code", quoted(table.String())),
		Why: fmt.Sprintf(
			"During a rolling deploy, instances running the previous release still query %s. Those queries fail as soon as this migration runs.",
			quoted(table.Name),
		),
		Lock: lockmodel.Impact(lockmodel.RenameTable, table),
		Fix: &ir.Fix{
			Summary:   "Keep the old name working with a view while code moves to the new name:",
			Framework: langSQL,
			Steps: []ir.FixStep{
				{
					Title: "Rename the table and create a view with the old name in the same transaction",
					Lang:  langSQL,
					Code:  fmt.Sprintf("ALTER TABLE %s RENAME TO %s;\nCREATE VIEW %s AS SELECT * FROM %s;", table, renamed.Name, table, renamed),
				},
				{Title: fmt.Sprintf("Change the application to use %s and deploy", renamed.Name)},
				{Title: "Drop the view in a later deploy"},
			},
		},
	}

	if span, ok := node.IdentifierSpan("TABLE", table.Name); ok {
		finding.Location.Span = span
	}

	return []ir.Finding{finding}
}
