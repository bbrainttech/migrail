package postgres

import (
	_ "embed"
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr403.md
var mr403Docs string

type missingLockTimeout struct{}

func (missingLockTimeout) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR403",
		Slug:            "missing-lock-timeout",
		Title:           "Locking DDL runs without a lock_timeout",
		Category:        ir.CategoryTransaction,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr403Docs,
	}
}

func (r missingLockTimeout) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	if value, ok := c.Setting("lock_timeout"); ok && !isDisabledTimeout(value) {
		return nil
	}

	node, ok := pg.NodeOf(stmt)
	if !ok {
		return nil
	}

	table, locks := lockedTable(c, node.Stmt)
	if !locks || !c.FirstInMigration(r.Meta().ID) {
		return nil
	}

	return []ir.Finding{{
		Confidence: ir.ConfidenceLikely,
		Title:      "No lock_timeout before locking DDL",
		Why: fmt.Sprintf(
			"If a long query holds a lock on %s, this statement waits for it, and every query that arrives afterwards queues behind this statement.",
			table,
		),
		Fix: &ir.Fix{
			Summary:   "Set a lock timeout at the top of the migration, and retry the migration if it times out:",
			Framework: langSQL,
			Steps:     []ir.FixStep{{Lang: langSQL, Code: "SET lock_timeout = '5s';"}},
		},
	}}
}

func isDisabledTimeout(value string) bool {
	trimmed := strings.TrimSpace(strings.ToLower(value))

	return trimmed == "" || trimmed == "0" || trimmed == "0s" || trimmed == "0ms"
}

func lockedTable(c *analyze.Context, stmt *pg_query.Node) (ir.ObjectRef, bool) {
	var tables []ir.ObjectRef

	switch {
	case stmt.GetAlterTableStmt() != nil && stmt.GetAlterTableStmt().GetObjtype() == pg_query.ObjectType_OBJECT_TABLE:
		tables = []ir.ObjectRef{pg.RelationRef(stmt.GetAlterTableStmt().GetRelation())}
	case stmt.GetIndexStmt() != nil && !stmt.GetIndexStmt().GetConcurrent():
		tables = []ir.ObjectRef{pg.RelationRef(stmt.GetIndexStmt().GetRelation())}
	case stmt.GetRenameStmt().GetRelation() != nil:
		tables = []ir.ObjectRef{pg.RelationRef(stmt.GetRenameStmt().GetRelation())}
	case stmt.GetCreateTrigStmt() != nil:
		tables = []ir.ObjectRef{pg.RelationRef(stmt.GetCreateTrigStmt().GetRelation())}
	case stmt.GetTruncateStmt() != nil:
		tables = relationsOf(stmt.GetTruncateStmt().GetRelations())
	case stmt.GetDropStmt().GetRemoveType() == pg_query.ObjectType_OBJECT_TABLE:
		tables = namesOf(stmt.GetDropStmt().GetObjects())
	}

	existing := existingTables(c, tables)
	if len(existing) == 0 {
		return ir.ObjectRef{}, false
	}

	return existing[0], true
}
