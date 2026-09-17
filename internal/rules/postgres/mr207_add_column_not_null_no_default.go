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

//go:embed mr207.md
var mr207Docs string

var serialTypes = []string{"serial", "serial2", "serial4", "serial8", "smallserial", "bigserial"}

type addColumnNotNullNoDefault struct{}

func (addColumnNotNullNoDefault) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR207",
		Slug:            "add-column-not-null-no-default",
		Title:           "Adding a NOT NULL column without a default fails on tables with rows",
		Category:        ir.CategoryRewrite,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr207Docs,
	}
}

func (addColumnNotNullNoDefault) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	alter, ok := existingAlterTable(c, stmt)
	if !ok {
		return nil
	}

	findings := []ir.Finding{}

	for _, cmd := range alter.cmds {
		if cmd.GetSubtype() != pg_query.AlterTableType_AT_AddColumn {
			continue
		}

		column := cmd.GetDef().GetColumnDef()
		if !needsValue(column) {
			continue
		}

		finding := notNullColumnFinding(alter.table, column)
		alter.spanAt(&finding, column.GetLocation())
		findings = append(findings, finding)
	}

	return findings
}

func needsValue(column *pg_query.ColumnDef) bool {
	notNull := len(columnConstraints(column, pg_query.ConstrType_CONSTR_NOTNULL, pg_query.ConstrType_CONSTR_PRIMARY)) > 0
	hasValue := len(columnConstraints(column,
		pg_query.ConstrType_CONSTR_DEFAULT, pg_query.ConstrType_CONSTR_IDENTITY, pg_query.ConstrType_CONSTR_GENERATED)) > 0

	return notNull && !hasValue && !slices.Contains(serialTypes, pg.TypeBaseName(column.GetTypeName()))
}

func notNullColumnFinding(table ir.ObjectRef, column *pg_query.ColumnDef) ir.Finding {
	ref := ir.ColumnRef{Table: table, Name: column.GetColname()}
	finding := ir.Finding{
		Title: fmt.Sprintf("Adding NOT NULL column %s without a default fails if the table has rows", quoted(ref.String())),
		Why: fmt.Sprintf(
			"Existing rows of %s have no value for the new column, so Postgres rejects the statement with \"contains null values\". "+
				"It passes on an empty development database and fails in production.",
			table,
		),
		Lock: lockmodel.Impact(lockmodel.AddColumn, table),
		Fix: &ir.Fix{
			Summary:   "Add the column as nullable, backfill it, then set NOT NULL without a long lock:",
			Framework: langSQL,
		},
	}

	addSQL, err := alterTableSQL(table, addColumnCmd(withoutConstraints(column, pg_query.ConstrType_CONSTR_NOTNULL)))
	if err != nil {
		finding.Fix.Steps = []ir.FixStep{{Title: "Add the column as nullable, backfill it, then set NOT NULL"}}

		return finding
	}

	finding.Fix.Steps = []ir.FixStep{
		{Title: "Add the column as nullable", Lang: langSQL, Code: addSQL},
		{Title: fmt.Sprintf("Backfill %s for existing rows in batches", column.GetColname())},
		{Title: "Set NOT NULL using a validated CHECK constraint first (see migrail explain MR107)"},
	}

	return finding
}
