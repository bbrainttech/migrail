package postgres

import (
	_ "embed"
	"fmt"
	"slices"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr201.md
var mr201Docs string

var possiblyCoercibleTargets = []string{"text", "varchar", "numeric", "inet", "timestamptz", "citext", "varbit"}

type alterColumnTypeRewrite struct{}

func (alterColumnTypeRewrite) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR201",
		Slug:            "alter-column-type-rewrite",
		Title:           "Changing a column type rewrites the table",
		Category:        ir.CategoryRewrite,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr201Docs,
	}
}

func (alterColumnTypeRewrite) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetAlterTableStmt() == nil {
		return nil
	}

	alter := node.Stmt.GetAlterTableStmt()
	table := pg.RelationRef(alter.GetRelation())

	if c.IsNewTable(table) {
		return nil
	}

	findings := []ir.Finding{}

	for _, cmdNode := range alter.GetCmds() {
		cmd := cmdNode.GetAlterTableCmd()
		if cmd.GetSubtype() != pg_query.AlterTableType_AT_AlterColumnType {
			continue
		}

		column := ir.ColumnRef{Table: table, Name: cmd.GetName()}
		findings = append(findings, typeChangeFinding(column, cmd.GetDef().GetColumnDef()))
	}

	return findings
}

func typeChangeFinding(column ir.ColumnRef, def *pg_query.ColumnDef) ir.Finding {
	usingForcesRewrite := def.GetRawDefault() != nil && !isColumnReference(def.GetRawDefault(), column.Name)
	possiblyCoercible := !usingForcesRewrite && slices.Contains(possiblyCoercibleTargets, pg.TypeBaseName(def.GetTypeName()))

	finding := ir.Finding{
		Severity:   ir.SeverityError,
		Confidence: ir.ConfidenceLikely,
		Title:      fmt.Sprintf("Changing the type of %s rewrites the table", quoted(column.String())),
		Why: fmt.Sprintf(
			"Postgres rewrites every row of %s and rebuilds its indexes while it holds an ACCESS EXCLUSIVE lock. Reads and writes wait until the rewrite finishes.",
			column.Table,
		),
		Lock: lockmodel.Impact(lockmodel.AlterColumnType, column.Table),
		Fix: &ir.Fix{
			Summary:   "Change the type with expand and contract instead of rewriting the table in place:",
			Framework: langSQL,
			Steps:     typeChangeSteps(column, def.GetTypeName()),
		},
	}

	switch {
	case usingForcesRewrite:
		finding.Confidence = ir.ConfidenceDefinite
	case possiblyCoercible:
		finding.Severity = ir.SeverityWarning
		finding.Confidence = ir.ConfidencePossible
		finding.Title = fmt.Sprintf("Changing the type of %s may rewrite the table", quoted(column.String()))
		finding.Why = fmt.Sprintf(
			"Some changes to this type only update the catalog, such as varchar to text or a longer varchar limit. "+
				"Other changes rewrite every row of %s under an ACCESS EXCLUSIVE lock. "+
				"migrail can't see the current column type in this migration.",
			column.Table,
		)
	}

	return finding
}

func isColumnReference(expr *pg_query.Node, column string) bool {
	fields := expr.GetColumnRef().GetFields()

	return len(fields) == 1 && fields[0].GetString_().GetSval() == column
}

func typeChangeSteps(column ir.ColumnRef, typeName *pg_query.TypeName) []ir.FixStep {
	newColumn := column.Name + "_new"
	addColumn, err := alterTableSQL(column.Table, addColumnCmd(&pg_query.ColumnDef{Colname: newColumn, TypeName: typeName}))

	first := ir.FixStep{Title: fmt.Sprintf("Add %s with the new type", newColumn)}
	if err == nil {
		first.Lang = langSQL
		first.Code = addColumn
	}

	return []ir.FixStep{
		first,
		{Title: fmt.Sprintf("Write to both columns and backfill %s in batches", newColumn)},
		{Title: fmt.Sprintf("Read from %s, then drop %s and rename %s", newColumn, column.Name, newColumn)},
	}
}
