package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"google.golang.org/protobuf/proto"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr104.md
var mr104Docs string

type addForeignKeyValidating struct{}

func (addForeignKeyValidating) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR104",
		Slug:            "add-foreign-key-validating",
		Title:           "Adding a foreign key without NOT VALID blocks writes on both tables",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr104Docs,
	}
}

func (addForeignKeyValidating) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
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

		var (
			finding  ir.Finding
			location int32
			found    bool
		)

		switch cmd.GetSubtype() {
		case pg_query.AlterTableType_AT_AddConstraint:
			location = cmd.GetDef().GetConstraint().GetLocation()
			finding, found = tableForeignKeyFinding(table, cmd.GetDef().GetConstraint())
		case pg_query.AlterTableType_AT_AddColumn:
			location = cmd.GetDef().GetColumnDef().GetLocation()
			finding, found = columnForeignKeyFinding(table, cmd.GetDef().GetColumnDef())
		default:
		}

		if !found {
			continue
		}

		if span, ok := node.ClauseSpan(location); ok {
			finding.Location.Span = span
		}

		findings = append(findings, finding)
	}

	return findings
}

func isValidatingForeignKey(constraint *pg_query.Constraint) bool {
	return constraint.GetContype() == pg_query.ConstrType_CONSTR_FOREIGN && !constraint.GetSkipValidation()
}

func tableForeignKeyFinding(table ir.ObjectRef, constraint *pg_query.Constraint) (ir.Finding, bool) {
	if !isValidatingForeignKey(constraint) {
		return ir.Finding{}, false
	}

	notValid, ok := proto.Clone(constraint).(*pg_query.Constraint)
	if !ok {
		return ir.Finding{}, false
	}

	if notValid.GetConname() == "" {
		notValid.Conname = pg.DefaultConstraintName(table.Name, pg.StringValues(constraint.GetFkAttrs()), "fkey")
	}

	notValid.SkipValidation = true
	notValid.InitiallyValid = false

	addSQL, addErr := alterTableSQL(table, addConstraintCmd(notValid))
	validateSQL, validateErr := alterTableSQL(table, namedCmd(pg_query.AlterTableType_AT_ValidateConstraint, notValid.GetConname()))

	finding := foreignKeyFinding(table, pg.RelationRef(constraint.GetPktable()))
	if addErr == nil && validateErr == nil {
		finding.Fix.Steps = []ir.FixStep{
			{Title: "Add the constraint without checking existing rows", Lang: langSQL, Code: addSQL},
			{Title: "Validate it in a separate migration", Lang: langSQL, Code: validateSQL},
		}
	}

	return finding, true
}

func columnForeignKeyFinding(table ir.ObjectRef, column *pg_query.ColumnDef) (ir.Finding, bool) {
	var foreignKey *pg_query.Constraint

	plain, ok := proto.Clone(column).(*pg_query.ColumnDef)
	if !ok {
		return ir.Finding{}, false
	}

	plain.Constraints = nil

	for _, constraintNode := range column.GetConstraints() {
		if constraint := constraintNode.GetConstraint(); isValidatingForeignKey(constraint) {
			foreignKey = constraint

			continue
		}

		plain.Constraints = append(plain.Constraints, constraintNode)
	}

	if foreignKey == nil {
		return ir.Finding{}, false
	}

	finding := foreignKeyFinding(table, pg.RelationRef(foreignKey.GetPktable()))

	if steps, err := inlineForeignKeySteps(table, plain, foreignKey); err == nil {
		finding.Fix.Steps = steps
	}

	return finding, true
}

func inlineForeignKeySteps(table ir.ObjectRef, column *pg_query.ColumnDef, foreignKey *pg_query.Constraint) ([]ir.FixStep, error) {
	constraint := &pg_query.Constraint{
		Contype:        pg_query.ConstrType_CONSTR_FOREIGN,
		Conname:        pg.DefaultConstraintName(table.Name, []string{column.GetColname()}, "fkey"),
		FkAttrs:        []*pg_query.Node{pg_query.MakeStrNode(column.GetColname())},
		Pktable:        foreignKey.GetPktable(),
		PkAttrs:        foreignKey.GetPkAttrs(),
		FkMatchtype:    foreignKey.GetFkMatchtype(),
		FkUpdAction:    foreignKey.GetFkUpdAction(),
		FkDelAction:    foreignKey.GetFkDelAction(),
		SkipValidation: true,
	}

	addColumn, err := alterTableSQL(table, addColumnCmd(column))
	if err != nil {
		return nil, err
	}

	addConstraint, err := alterTableSQL(table, addConstraintCmd(constraint))
	if err != nil {
		return nil, err
	}

	validate, err := alterTableSQL(table, namedCmd(pg_query.AlterTableType_AT_ValidateConstraint, constraint.GetConname()))
	if err != nil {
		return nil, err
	}

	return []ir.FixStep{
		{Title: "Add the column and the constraint without checking existing rows", Lang: langSQL, Code: joinSQL(addColumn, addConstraint)},
		{Title: "Validate it in a separate migration", Lang: langSQL, Code: validate},
	}, nil
}

func foreignKeyFinding(table, referenced ir.ObjectRef) ir.Finding {
	return ir.Finding{
		Title: fmt.Sprintf("Foreign key blocks writes on %s and %s", quoted(table.String()), quoted(referenced.String())),
		Why: fmt.Sprintf(
			"Postgres checks every existing row of %s against %s while it holds locks on both tables. "+
				"INSERT, UPDATE and DELETE on either table wait until the check finishes.",
			table, referenced,
		),
		Lock: lockmodel.Impact(lockmodel.AddForeignKey, table, referenced),
		Fix: &ir.Fix{
			Summary:   "Add the constraint as NOT VALID, then validate it in a separate transaction:",
			Framework: langSQL,
		},
	}
}
