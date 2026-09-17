package postgres

import (
	_ "embed"
	"fmt"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr503.md
var mr503Docs string

type deleteOrUpdateWithoutWhere struct{}

func (deleteOrUpdateWithoutWhere) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR503",
		Slug:            "delete-or-update-without-where",
		Title:           "UPDATE or DELETE without WHERE changes every row",
		Category:        ir.CategoryData,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr503Docs,
	}
}

func (deleteOrUpdateWithoutWhere) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok {
		return nil
	}

	command, verb := "", ""

	var table ir.ObjectRef

	switch {
	case node.Stmt.GetUpdateStmt() != nil && node.Stmt.GetUpdateStmt().GetWhereClause() == nil:
		command, verb = "UPDATE", "changes"
		table = pg.RelationRef(node.Stmt.GetUpdateStmt().GetRelation())
	case node.Stmt.GetDeleteStmt() != nil && node.Stmt.GetDeleteStmt().GetWhereClause() == nil:
		command, verb = "DELETE", "deletes"
		table = pg.RelationRef(node.Stmt.GetDeleteStmt().GetRelation())
	default:
		return nil
	}

	if c.IsNewTable(table) {
		return nil
	}

	return []ir.Finding{{
		Title: fmt.Sprintf("%s without WHERE %s every row in %s", command, verb, quoted(table.String())),
		Why: "One statement touches the whole table and holds row locks on every row until the transaction commits. " +
			"Concurrent writes to those rows wait, and the table bloats.",
		Fix: &ir.Fix{
			Summary:   "Add a WHERE clause, or process the table in batches outside the schema migration:",
			Framework: langSQL,
			Steps: []ir.FixStep{
				{Title: "Limit the statement to the rows it should change"},
				{Title: "For a full backfill, update rows in batches by primary key range in a separate migration or job"},
			},
		},
	}}
}
