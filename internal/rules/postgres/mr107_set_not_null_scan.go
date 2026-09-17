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

//go:embed mr107.md
var mr107Docs string

type setNotNullScan struct{}

func (setNotNullScan) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR107",
		Slug:            "set-not-null-scan",
		Title:           "SET NOT NULL scans the table while blocking reads and writes",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr107Docs,
	}
}

func (setNotNullScan) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
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
		if cmd.GetSubtype() != pg_query.AlterTableType_AT_SetNotNull {
			continue
		}

		column := ir.ColumnRef{Table: table, Name: cmd.GetName()}
		if c.HasValidatedNotNullCheck(column) {
			continue
		}

		findings = append(findings, setNotNullFinding(column))
	}

	return findings
}

func setNotNullFinding(column ir.ColumnRef) ir.Finding {
	finding := ir.Finding{
		Title: fmt.Sprintf("SET NOT NULL on %s blocks reads and writes during a full table scan", quoted(column.String())),
		Why: fmt.Sprintf(
			"Postgres scans all of %s to check for NULL values while it holds an ACCESS EXCLUSIVE lock. Every query on the table waits for the scan.",
			column.Table,
		),
		Lock: lockmodel.Impact(lockmodel.SetNotNull, column.Table),
		Fix: &ir.Fix{
			Summary:   "Prove the column has no NULL values with a CHECK constraint first, so SET NOT NULL skips the scan:",
			Framework: langSQL,
		},
	}

	if steps, err := setNotNullSteps(column); err == nil {
		finding.Fix.Steps = steps
	}

	return finding
}

func setNotNullSteps(column ir.ColumnRef) ([]ir.FixStep, error) {
	name := pg.DefaultConstraintName(column.Table.Name, []string{column.Name}, "not_null")

	addCheck, err := alterTableSQL(column.Table, addConstraintCmd(notNullCheck(name, column.Name)))
	if err != nil {
		return nil, err
	}

	validate, err := alterTableSQL(column.Table, namedCmd(pg_query.AlterTableType_AT_ValidateConstraint, name))
	if err != nil {
		return nil, err
	}

	setNotNull, err := alterTableSQL(column.Table, namedCmd(pg_query.AlterTableType_AT_SetNotNull, column.Name))
	if err != nil {
		return nil, err
	}

	dropCheck, err := alterTableSQL(column.Table, namedCmd(pg_query.AlterTableType_AT_DropConstraint, name))
	if err != nil {
		return nil, err
	}

	return []ir.FixStep{
		{Title: "Add the check without scanning existing rows", Lang: langSQL, Code: addCheck},
		{Title: stepValidateSeparately, Lang: langSQL, Code: validate},
		{Title: "Set NOT NULL, then drop the check", Lang: langSQL, Code: joinSQL(setNotNull, dropCheck)},
	}, nil
}
