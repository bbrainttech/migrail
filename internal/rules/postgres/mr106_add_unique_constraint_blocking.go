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

//go:embed mr106.md
var mr106Docs string

type addUniqueConstraintBlocking struct{}

func (addUniqueConstraintBlocking) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR106",
		Slug:            "add-unique-constraint-blocking",
		Title:           "Adding UNIQUE or PRIMARY KEY builds an index while blocking reads and writes",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr106Docs,
	}
}

func (addUniqueConstraintBlocking) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	alter, ok := existingAlterTable(c, stmt)
	if !ok {
		return nil
	}

	findings := []ir.Finding{}

	for _, cmd := range alter.cmds {
		constraint := cmd.GetDef().GetConstraint()
		kind := constraint.GetContype()

		buildsIndex := cmd.GetSubtype() == pg_query.AlterTableType_AT_AddConstraint &&
			(kind == pg_query.ConstrType_CONSTR_UNIQUE || kind == pg_query.ConstrType_CONSTR_PRIMARY) &&
			constraint.GetIndexname() == ""
		if !buildsIndex {
			continue
		}

		finding := uniqueFinding(alter.table, constraint)
		alter.spanAt(&finding, constraint.GetLocation())
		findings = append(findings, finding)
	}

	return findings
}

func uniqueFinding(table ir.ObjectRef, constraint *pg_query.Constraint) ir.Finding {
	kind, label := "UNIQUE", "key"
	if constraint.GetContype() == pg_query.ConstrType_CONSTR_PRIMARY {
		kind, label = "PRIMARY KEY", "pkey"
	}

	columns := pg.StringValues(constraint.GetKeys())

	finding := ir.Finding{
		Title: fmt.Sprintf("Adding %s on %s builds an index while blocking reads and writes", kind, quoted(table.String())),
		Why: fmt.Sprintf(
			"Postgres builds the index for this constraint while it holds an ACCESS EXCLUSIVE lock on %s. Every query on the table waits for the whole build.",
			table,
		),
		Lock: lockmodel.Impact(lockmodel.AddUnique, table),
		Fix:  &ir.Fix{Summary: "Build the index concurrently, then attach it to the constraint:", Framework: langSQL},
	}

	name := constraint.GetConname()
	if name == "" {
		name = pg.DefaultConstraintName(table.Name, columns, label)
		if label == "pkey" {
			name = pg.DefaultConstraintName(table.Name, nil, label)
		}
	}

	if steps, err := uniqueSteps(table, constraint, name, columns); err == nil {
		finding.Fix.Steps = steps
	}

	return finding
}

func uniqueSteps(table ir.ObjectRef, constraint *pg_query.Constraint, name string, columns []string) ([]ir.FixStep, error) {
	params := make([]*pg_query.Node, 0, len(columns))
	for _, column := range columns {
		params = append(params, &pg_query.Node{Node: &pg_query.Node_IndexElem{IndexElem: &pg_query.IndexElem{
			Name:          column,
			Ordering:      pg_query.SortByDir_SORTBY_DEFAULT,
			NullsOrdering: pg_query.SortByNulls_SORTBY_NULLS_DEFAULT,
		}}})
	}

	index := &pg_query.IndexStmt{
		Idxname:     name,
		Relation:    pg.RangeVar(table),
		IndexParams: params,
		Unique:      true,
		Concurrent:  true,
	}

	indexSQL, err := pg.Deparse(&pg_query.Node{Node: &pg_query.Node_IndexStmt{IndexStmt: index}})
	if err != nil {
		return nil, err
	}

	attached := &pg_query.Constraint{Contype: constraint.GetContype(), Conname: name, Indexname: name}

	attachSQL, err := alterTableSQL(table, addConstraintCmd(attached))
	if err != nil {
		return nil, err
	}

	return []ir.FixStep{
		{Title: "Build a unique index concurrently, outside a transaction", Lang: langSQL, Code: indexSQL + ";"},
		{Title: "Add the constraint using that index", Lang: langSQL, Code: attachSQL},
	}, nil
}
