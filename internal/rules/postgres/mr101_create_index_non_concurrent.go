package postgres

import (
	_ "embed"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr101.md
var mr101Docs string

type createIndexNonConcurrent struct{}

func (createIndexNonConcurrent) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR101",
		Slug:            "create-index-non-concurrent",
		Title:           "CREATE INDEX without CONCURRENTLY blocks writes",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr101Docs,
	}
}

func (createIndexNonConcurrent) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetIndexStmt() == nil {
		return nil
	}

	index := node.Stmt.GetIndexStmt()
	table := pg.RelationRef(index.GetRelation())

	if index.GetConcurrent() || c.IsNewTable(table) {
		return nil
	}

	return []ir.Finding{{
		Title: "Index creation blocks writes on " + quoted(table.String()),
		Why:   "Postgres holds this lock until the whole index is built. INSERT, UPDATE and DELETE on the table wait for the entire build.",
		Lock:  lockmodel.Impact(lockmodel.CreateIndex, table),
		Fix: &ir.Fix{
			Summary:   "Build the index concurrently, outside a transaction:",
			Framework: langSQL,
			Steps:     []ir.FixStep{{Lang: langSQL, Code: concurrentIndexSQL(node, index)}},
		},
	}}
}

func concurrentIndexSQL(node *pg.Node, index *pg_query.IndexStmt) string {
	if sql, ok := node.InsertAfterKeyword("INDEX", " CONCURRENTLY"); ok {
		return sql + ";"
	}

	concurrent, ok := proto.Clone(index).(*pg_query.IndexStmt)
	if !ok {
		return ""
	}

	concurrent.Concurrent = true

	sql, err := pg.Deparse(&pg_query.Node{Node: &pg_query.Node_IndexStmt{IndexStmt: concurrent}})
	if err != nil {
		return ""
	}

	return sql + ";"
}
