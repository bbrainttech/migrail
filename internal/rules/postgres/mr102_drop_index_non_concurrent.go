package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr102.md
var mr102Docs string

type dropIndexNonConcurrent struct{}

func (dropIndexNonConcurrent) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR102",
		Slug:            "drop-index-non-concurrent",
		Title:           "DROP INDEX without CONCURRENTLY blocks reads and writes",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr102Docs,
	}
}

func (dropIndexNonConcurrent) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetDropStmt() == nil {
		return nil
	}

	drop := node.Stmt.GetDropStmt()
	if drop.GetRemoveType() != pg_query.ObjectType_OBJECT_INDEX || drop.GetConcurrent() {
		return nil
	}

	findings := []ir.Finding{}

	for _, index := range namesOf(drop.GetObjects()) {
		if c.IsNewIndex(index) {
			continue
		}

		finding := ir.Finding{
			Title: fmt.Sprintf("Dropping index %s blocks reads and writes on its table", quoted(index.String())),
			Why: "DROP INDEX takes an ACCESS EXCLUSIVE lock on the table the index belongs to. " +
				"If a long query is using the table, the drop waits, and every new query on the table queues behind it.",
			Fix: &ir.Fix{Summary: "Drop the index concurrently, outside a transaction:", Framework: langSQL},
		}

		if sql, err := dropIndexConcurrentlySQL(index, drop.GetMissingOk()); err == nil {
			finding.Fix.Steps = []ir.FixStep{{Lang: langSQL, Code: sql}}
		}

		if span, ok := node.IdentifierSpan("INDEX", index.Name); ok {
			finding.Location.Span = span
		}

		findings = append(findings, finding)
	}

	return findings
}

func dropIndexConcurrentlySQL(index ir.ObjectRef, missingOK bool) (string, error) {
	names := []*pg_query.Node{}
	if index.Schema != "" {
		names = append(names, pg_query.MakeStrNode(index.Schema))
	}

	names = append(names, pg_query.MakeStrNode(index.Name))

	stmt := &pg_query.DropStmt{
		Objects:    []*pg_query.Node{pg_query.MakeListNode(names)},
		RemoveType: pg_query.ObjectType_OBJECT_INDEX,
		Behavior:   pg_query.DropBehavior_DROP_RESTRICT,
		MissingOk:  missingOK,
		Concurrent: true,
	}

	sql, err := pg.Deparse(&pg_query.Node{Node: &pg_query.Node_DropStmt{DropStmt: stmt}})
	if err != nil {
		return "", err
	}

	return sql + ";", nil
}
