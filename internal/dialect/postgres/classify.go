package postgres

import (
	"strconv"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/ir"
)

func classify(stmt *ir.Statement, node *pg_query.Node) {
	switch {
	case node.GetTransactionStmt() != nil:
		stmt.Kind = transactionKind(node.GetTransactionStmt().GetKind())
	case node.GetVariableSetStmt() != nil:
		classifySet(stmt, node.GetVariableSetStmt())
	case node.GetCreateStmt() != nil:
		create := node.GetCreateStmt()
		classifyCreateTable(stmt, RelationRef(create.GetRelation()), create.GetIfNotExists())
	case node.GetCreateTableAsStmt() != nil:
		create := node.GetCreateTableAsStmt()
		classifyCreateTable(stmt, RelationRef(create.GetInto().GetRel()), create.GetIfNotExists())
	case node.GetIndexStmt() != nil:
		classifyCreateIndex(stmt, node.GetIndexStmt())
	case node.GetAlterTableStmt() != nil:
		classifyAlterTable(stmt, node.GetAlterTableStmt())
	case node.GetRenameStmt() != nil:
		classifyRename(stmt, node.GetRenameStmt())
	case node.GetDropStmt() != nil:
		classifyDrop(stmt, node.GetDropStmt())
	}
}

func transactionKind(kind pg_query.TransactionStmtKind) ir.StmtKind {
	switch kind {
	case pg_query.TransactionStmtKind_TRANS_STMT_BEGIN, pg_query.TransactionStmtKind_TRANS_STMT_START:
		return ir.StmtBegin
	case pg_query.TransactionStmtKind_TRANS_STMT_COMMIT, pg_query.TransactionStmtKind_TRANS_STMT_PREPARE:
		return ir.StmtCommit
	case pg_query.TransactionStmtKind_TRANS_STMT_ROLLBACK:
		return ir.StmtRollback
	default:
		return ir.StmtUnknown
	}
}

func classifySet(stmt *ir.Statement, set *pg_query.VariableSetStmt) {
	setting := &ir.Setting{Name: set.GetName(), Local: set.GetIsLocal()}

	switch set.GetKind() {
	case pg_query.VariableSetKind_VAR_SET_VALUE:
		stmt.Kind = ir.StmtSet
		setting.Value = settingValue(set.GetArgs())
	case pg_query.VariableSetKind_VAR_SET_DEFAULT, pg_query.VariableSetKind_VAR_RESET:
		stmt.Kind = ir.StmtReset
	default:
		stmt.Kind = ir.StmtUnknown
	}

	stmt.Setting = setting
}

func settingValue(args []*pg_query.Node) string {
	if len(args) == 0 {
		return ""
	}

	constant := args[0].GetAConst()

	switch {
	case constant.GetSval() != nil:
		return constant.GetSval().GetSval()
	case constant.GetIval() != nil:
		return strconv.Itoa(int(constant.GetIval().GetIval()))
	case constant.GetFval() != nil:
		return constant.GetFval().GetFval()
	default:
		return ""
	}
}

func classifyCreateTable(stmt *ir.Statement, table ir.ObjectRef, ifNotExists bool) {
	stmt.Kind = ir.StmtCreateTable
	stmt.Targets = []ir.ObjectRef{table}

	if ifNotExists {
		return
	}

	stmt.Effects = append(stmt.Effects, ir.Effect{Kind: ir.EffectCreateTable, Object: table})
}

func classifyCreateIndex(stmt *ir.Statement, index *pg_query.IndexStmt) {
	table := RelationRef(index.GetRelation())
	stmt.Kind = ir.StmtCreateIndex
	stmt.Targets = []ir.ObjectRef{table}

	if index.GetIdxname() == "" || index.GetIfNotExists() {
		return
	}

	stmt.Effects = append(stmt.Effects, ir.Effect{
		Kind:   ir.EffectCreateIndex,
		Object: ir.ObjectRef{Schema: table.Schema, Name: index.GetIdxname()},
	})
}

func classifyAlterTable(stmt *ir.Statement, alter *pg_query.AlterTableStmt) {
	table := RelationRef(alter.GetRelation())
	stmt.Kind = ir.StmtAlterTable
	stmt.Targets = []ir.ObjectRef{table}

	for _, cmdNode := range alter.GetCmds() {
		cmd := cmdNode.GetAlterTableCmd()

		if column := commandColumn(cmd); column != "" {
			stmt.Columns = append(stmt.Columns, ir.ColumnRef{Table: table, Name: column})
		}

		if effect, ok := commandEffect(table, cmd); ok {
			stmt.Effects = append(stmt.Effects, effect)
		}

		if effect, ok := addConstraintEffect(table, cmd); ok {
			stmt.Effects = append(stmt.Effects, effect)
		}
	}
}

func commandColumn(cmd *pg_query.AlterTableCmd) string {
	if cmd.GetSubtype() == pg_query.AlterTableType_AT_AddColumn {
		return cmd.GetDef().GetColumnDef().GetColname()
	}

	switch cmd.GetSubtype() {
	case pg_query.AlterTableType_AT_AlterColumnType,
		pg_query.AlterTableType_AT_SetNotNull,
		pg_query.AlterTableType_AT_DropNotNull,
		pg_query.AlterTableType_AT_DropColumn,
		pg_query.AlterTableType_AT_ColumnDefault:
		return cmd.GetName()
	default:
		return ""
	}
}

func commandEffect(table ir.ObjectRef, cmd *pg_query.AlterTableCmd) (ir.Effect, bool) {
	switch cmd.GetSubtype() {
	case pg_query.AlterTableType_AT_ValidateConstraint:
		return ir.Effect{Kind: ir.EffectValidateConstraint, Object: table, Constraint: cmd.GetName()}, true
	case pg_query.AlterTableType_AT_DropConstraint:
		return ir.Effect{Kind: ir.EffectDropConstraint, Object: table, Constraint: cmd.GetName()}, true
	case pg_query.AlterTableType_AT_AddConstraint:
		return notNullCheckEffect(table, cmd.GetDef().GetConstraint())
	default:
		return ir.Effect{}, false
	}
}

func addConstraintEffect(table ir.ObjectRef, cmd *pg_query.AlterTableCmd) (ir.Effect, bool) {
	if cmd.GetSubtype() != pg_query.AlterTableType_AT_AddConstraint {
		return ir.Effect{}, false
	}

	name := ConstraintName(table, cmd.GetDef().GetConstraint())
	if name == "" {
		return ir.Effect{}, false
	}

	return ir.Effect{
		Kind:       ir.EffectAddConstraint,
		Object:     table,
		Constraint: name,
		Validated:  !cmd.GetDef().GetConstraint().GetSkipValidation(),
	}, true
}

func ConstraintName(table ir.ObjectRef, constraint *pg_query.Constraint) string {
	if constraint.GetConname() != "" {
		return constraint.GetConname()
	}

	switch constraint.GetContype() {
	case pg_query.ConstrType_CONSTR_FOREIGN:
		return DefaultConstraintName(table.Name, StringValues(constraint.GetFkAttrs()), "fkey")
	case pg_query.ConstrType_CONSTR_CHECK:
		if column, ok := notNullColumn(constraint); ok {
			return DefaultConstraintName(table.Name, []string{column}, "check")
		}
	default:
	}

	return ""
}

func notNullColumn(constraint *pg_query.Constraint) (string, bool) {
	test := constraint.GetRawExpr().GetNullTest()
	if test == nil || test.GetNulltesttype() != pg_query.NullTestType_IS_NOT_NULL {
		return "", false
	}

	fields := test.GetArg().GetColumnRef().GetFields()
	if len(fields) != 1 || fields[0].GetString_() == nil {
		return "", false
	}

	return fields[0].GetString_().GetSval(), true
}

func notNullCheckEffect(table ir.ObjectRef, constraint *pg_query.Constraint) (ir.Effect, bool) {
	if constraint.GetContype() != pg_query.ConstrType_CONSTR_CHECK {
		return ir.Effect{}, false
	}

	column, ok := notNullColumn(constraint)
	if !ok {
		return ir.Effect{}, false
	}

	name := ConstraintName(table, constraint)

	return ir.Effect{
		Kind:       ir.EffectAddNotNullCheck,
		Object:     table,
		Column:     column,
		Constraint: name,
		Validated:  !constraint.GetSkipValidation(),
	}, true
}

func classifyRename(stmt *ir.Statement, rename *pg_query.RenameStmt) {
	stmt.Kind = ir.StmtRename

	if rename.GetRelation() == nil {
		return
	}

	table := RelationRef(rename.GetRelation())
	stmt.Targets = []ir.ObjectRef{table}

	switch rename.GetRenameType() {
	case pg_query.ObjectType_OBJECT_COLUMN:
		stmt.Columns = []ir.ColumnRef{{Table: table, Name: rename.GetSubname()}}
	case pg_query.ObjectType_OBJECT_TABLE:
		stmt.Effects = append(stmt.Effects, ir.Effect{Kind: ir.EffectRenameTable, Object: table, NewName: rename.GetNewname()})
	default:
	}
}

func classifyDrop(stmt *ir.Statement, drop *pg_query.DropStmt) {
	switch drop.GetRemoveType() {
	case pg_query.ObjectType_OBJECT_TABLE:
		stmt.Kind = ir.StmtDropTable
	case pg_query.ObjectType_OBJECT_INDEX:
		stmt.Kind = ir.StmtDropIndex
	default:
		return
	}

	for _, object := range drop.GetObjects() {
		names := StringValues(object.GetList().GetItems())
		if len(names) == 0 {
			continue
		}

		ref := ir.ObjectRef{Name: names[len(names)-1]}
		if len(names) > 1 {
			ref.Schema = names[len(names)-2]
		}

		stmt.Targets = append(stmt.Targets, ref)

		kind := ir.EffectDropIndex
		if stmt.Kind == ir.StmtDropTable {
			kind = ir.EffectDropTable
		}

		stmt.Effects = append(stmt.Effects, ir.Effect{Kind: kind, Object: ref})
	}
}
