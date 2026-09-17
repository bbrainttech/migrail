package postgres

import (
	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

type alterTable struct {
	node  *pg.Node
	table ir.ObjectRef
	cmds  []*pg_query.AlterTableCmd
}

func existingAlterTable(c *analyze.Context, stmt *ir.Statement) (alterTable, bool) {
	node, ok := pg.NodeOf(stmt)
	if !ok || node.Stmt.GetAlterTableStmt() == nil {
		return alterTable{}, false
	}

	alter := node.Stmt.GetAlterTableStmt()
	if alter.GetObjtype() != pg_query.ObjectType_OBJECT_TABLE {
		return alterTable{}, false
	}

	table := pg.RelationRef(alter.GetRelation())
	if c.IsNewTable(table) {
		return alterTable{}, false
	}

	cmds := make([]*pg_query.AlterTableCmd, 0, len(alter.GetCmds()))
	for _, cmd := range alter.GetCmds() {
		cmds = append(cmds, cmd.GetAlterTableCmd())
	}

	return alterTable{node: node, table: table, cmds: cmds}, true
}

func (a alterTable) spanAt(finding *ir.Finding, location int32) {
	if span, ok := a.node.ClauseSpan(location); ok {
		finding.Location.Span = span
	}
}

func columnConstraints(column *pg_query.ColumnDef, kinds ...pg_query.ConstrType) []*pg_query.Constraint {
	found := []*pg_query.Constraint{}

	for _, node := range column.GetConstraints() {
		constraint := node.GetConstraint()

		for _, kind := range kinds {
			if constraint.GetContype() == kind {
				found = append(found, constraint)
			}
		}
	}

	return found
}

func withoutConstraints(column *pg_query.ColumnDef, kinds ...pg_query.ConstrType) *pg_query.ColumnDef {
	copied := &pg_query.ColumnDef{
		Colname:  column.GetColname(),
		TypeName: column.GetTypeName(),
		IsLocal:  column.GetIsLocal(),
	}

	for _, node := range column.GetConstraints() {
		keep := true

		for _, kind := range kinds {
			if node.GetConstraint().GetContype() == kind {
				keep = false
			}
		}

		if keep {
			copied.Constraints = append(copied.Constraints, node)
		}
	}

	return copied
}

func relationsOf(nodes []*pg_query.Node) []ir.ObjectRef {
	refs := []ir.ObjectRef{}

	for _, node := range nodes {
		switch {
		case node.GetRangeVar() != nil:
			refs = append(refs, pg.RelationRef(node.GetRangeVar()))
		case node.GetVacuumRelation() != nil && node.GetVacuumRelation().GetRelation() != nil:
			refs = append(refs, pg.RelationRef(node.GetVacuumRelation().GetRelation()))
		}
	}

	return refs
}

func existingTables(c *analyze.Context, tables []ir.ObjectRef) []ir.ObjectRef {
	existing := []ir.ObjectRef{}

	for _, table := range tables {
		if !c.IsNewTable(table) {
			existing = append(existing, table)
		}
	}

	return existing
}

func namesOf(nodes []*pg_query.Node) []ir.ObjectRef {
	refs := []ir.ObjectRef{}

	for _, object := range nodes {
		names := pg.StringValues(object.GetList().GetItems())
		if len(names) == 0 {
			continue
		}

		ref := ir.ObjectRef{Name: names[len(names)-1]}
		if len(names) > 1 {
			ref.Schema = names[len(names)-2]
		}

		refs = append(refs, ref)
	}

	return refs
}
