package postgres

import (
	_ "embed"
	"strings"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr501.md
var mr501Docs string

type truncateTable struct{}

func (truncateTable) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR501",
		Slug:            "truncate-table",
		Title:           "TRUNCATE deletes every row",
		Category:        ir.CategoryData,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr501Docs,
	}
}

func (truncateTable) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetTruncateStmt() == nil {
		return nil
	}

	tables := existingTables(c, relationsOf(node.Stmt.GetTruncateStmt().GetRelations()))
	if len(tables) == 0 {
		return nil
	}

	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, quoted(table.String()))
	}

	return []ir.Finding{{
		Title: "TRUNCATE deletes every row in " + strings.Join(names, ", "),
		Why:   "The data is gone as soon as the migration commits, in every environment the migration runs in. TRUNCATE also holds an ACCESS EXCLUSIVE lock.",
		Lock:  lockmodel.Impact(lockmodel.Truncate, tables...),
		Fix: &ir.Fix{
			Summary:   "Confirm this is intended. If it is, ignore the finding with a reason:",
			Framework: langSQL,
			Steps:     []ir.FixStep{{Lang: langSQL, Code: `-- migrail:ignore MR501 reason="explain why deleting this data is safe"`}},
		},
	}}
}
