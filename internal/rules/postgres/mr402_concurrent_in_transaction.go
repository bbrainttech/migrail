package postgres

import (
	_ "embed"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr402.md
var mr402Docs string

type concurrentInTransaction struct{}

func (concurrentInTransaction) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR402",
		Slug:            "concurrent-in-transaction",
		Title:           "CONCURRENTLY can't run inside a transaction",
		Category:        ir.CategoryTransaction,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr402Docs,
	}
}

func (concurrentInTransaction) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	if !c.InTransaction() {
		return nil
	}

	node, ok := pg.NodeOf(stmt)
	if !ok {
		return nil
	}

	command := concurrentCommand(node.Stmt)
	if command == "" {
		return nil
	}

	return []ir.Finding{{
		Title: command + " can't run inside a transaction",
		Why: "Postgres rejects " + command + " inside a transaction block with \"cannot run inside a transaction block\", " +
			"so the migration fails when it runs.",
		Fix: &ir.Fix{
			Summary:   "Run this statement outside a transaction:",
			Framework: langSQL,
			Steps: []ir.FixStep{
				{Title: "Move it to its own migration"},
				{Title: "Turn off the migration tool's transaction for that migration, or remove BEGIN and COMMIT around it"},
			},
		},
	}}
}

func concurrentCommand(stmt *pg_query.Node) string {
	switch {
	case stmt.GetIndexStmt().GetConcurrent():
		return "CREATE INDEX CONCURRENTLY"
	case stmt.GetDropStmt().GetConcurrent():
		return "DROP INDEX CONCURRENTLY"
	case stmt.GetReindexStmt() != nil && hasOption(stmt.GetReindexStmt().GetParams(), "concurrently"):
		return "REINDEX CONCURRENTLY"
	case stmt.GetRefreshMatViewStmt().GetConcurrent():
		return "REFRESH MATERIALIZED VIEW CONCURRENTLY"
	default:
		return ""
	}
}

func hasOption(options []*pg_query.Node, name string) bool {
	for _, option := range options {
		if option.GetDefElem().GetDefname() == name {
			return true
		}
	}

	return false
}
