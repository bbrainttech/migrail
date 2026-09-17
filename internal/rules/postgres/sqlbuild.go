package postgres

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

const langSQL = "sql"

func quoted(name string) string {
	return `"` + name + `"`
}

func alterTableSQL(table ir.ObjectRef, cmds ...*pg_query.AlterTableCmd) (string, error) {
	nodes := make([]*pg_query.Node, 0, len(cmds))
	for _, cmd := range cmds {
		nodes = append(nodes, &pg_query.Node{Node: &pg_query.Node_AlterTableCmd{AlterTableCmd: cmd}})
	}

	stmt := &pg_query.AlterTableStmt{
		Relation: pg.RangeVar(table),
		Cmds:     nodes,
		Objtype:  pg_query.ObjectType_OBJECT_TABLE,
	}

	sql, err := pg.Deparse(&pg_query.Node{Node: &pg_query.Node_AlterTableStmt{AlterTableStmt: stmt}})
	if err != nil {
		return "", fmt.Errorf("build ALTER TABLE for %s: %w", table, err)
	}

	return sql + ";", nil
}

func addConstraintCmd(constraint *pg_query.Constraint) *pg_query.AlterTableCmd {
	return &pg_query.AlterTableCmd{
		Subtype:  pg_query.AlterTableType_AT_AddConstraint,
		Def:      &pg_query.Node{Node: &pg_query.Node_Constraint{Constraint: constraint}},
		Behavior: pg_query.DropBehavior_DROP_RESTRICT,
	}
}

func addColumnCmd(column *pg_query.ColumnDef) *pg_query.AlterTableCmd {
	return &pg_query.AlterTableCmd{
		Subtype:  pg_query.AlterTableType_AT_AddColumn,
		Def:      &pg_query.Node{Node: &pg_query.Node_ColumnDef{ColumnDef: column}},
		Behavior: pg_query.DropBehavior_DROP_RESTRICT,
	}
}

func namedCmd(subtype pg_query.AlterTableType, name string) *pg_query.AlterTableCmd {
	return &pg_query.AlterTableCmd{Subtype: subtype, Name: name, Behavior: pg_query.DropBehavior_DROP_RESTRICT}
}

func notNullCheck(name, column string) *pg_query.Constraint {
	columnRef := &pg_query.ColumnRef{Fields: []*pg_query.Node{pg_query.MakeStrNode(column)}}
	test := &pg_query.NullTest{
		Arg:          &pg_query.Node{Node: &pg_query.Node_ColumnRef{ColumnRef: columnRef}},
		Nulltesttype: pg_query.NullTestType_IS_NOT_NULL,
	}

	return &pg_query.Constraint{
		Contype:        pg_query.ConstrType_CONSTR_CHECK,
		Conname:        name,
		RawExpr:        &pg_query.Node{Node: &pg_query.Node_NullTest{NullTest: test}},
		SkipValidation: true,
	}
}

func joinSQL(statements ...string) string {
	return strings.Join(statements, "\n")
}
