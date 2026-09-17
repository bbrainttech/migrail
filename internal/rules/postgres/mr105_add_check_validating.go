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

//go:embed mr105.md
var mr105Docs string

type addCheckValidating struct{}

func (addCheckValidating) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR105",
		Slug:            "add-check-validating",
		Title:           "Adding a CHECK constraint without NOT VALID blocks reads and writes",
		Category:        ir.CategoryLocking,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr105Docs,
	}
}

func (addCheckValidating) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	alter, ok := existingAlterTable(c, stmt)
	if !ok {
		return nil
	}

	findings := []ir.Finding{}

	for _, cmd := range alter.cmds {
		constraint := cmd.GetDef().GetConstraint()

		isValidatingCheck := cmd.GetSubtype() == pg_query.AlterTableType_AT_AddConstraint &&
			constraint.GetContype() == pg_query.ConstrType_CONSTR_CHECK && !constraint.GetSkipValidation()
		if !isValidatingCheck {
			continue
		}

		finding := checkFinding(alter.table, constraint)
		alter.spanAt(&finding, constraint.GetLocation())
		findings = append(findings, finding)
	}

	return findings
}

func checkFinding(table ir.ObjectRef, constraint *pg_query.Constraint) ir.Finding {
	finding := ir.Finding{
		Title: "CHECK constraint validation blocks reads and writes on " + quoted(table.String()),
		Why: fmt.Sprintf(
			"Postgres checks every existing row of %s while it holds an ACCESS EXCLUSIVE lock. Every query on the table waits until the check finishes.",
			table,
		),
		Lock: lockmodel.Impact(lockmodel.AddCheck, table),
		Fix:  &ir.Fix{Summary: "Add the constraint as NOT VALID, then validate it in a separate transaction:", Framework: langSQL},
	}

	notValid, ok := proto.Clone(constraint).(*pg_query.Constraint)
	if !ok {
		return finding
	}

	if notValid.GetConname() == "" {
		notValid.Conname = pg.ConstraintName(table, constraint)
	}

	if notValid.GetConname() == "" {
		notValid.Conname = pg.DefaultConstraintName(table.Name, nil, "check")
	}

	notValid.SkipValidation = true
	notValid.InitiallyValid = false

	addSQL, addErr := alterTableSQL(table, addConstraintCmd(notValid))
	validateSQL, validateErr := alterTableSQL(table, namedCmd(pg_query.AlterTableType_AT_ValidateConstraint, notValid.GetConname()))

	if addErr == nil && validateErr == nil {
		finding.Fix.Steps = []ir.FixStep{
			{Title: "Add the constraint without checking existing rows", Lang: langSQL, Code: addSQL},
			{Title: stepValidateSeparately, Lang: langSQL, Code: validateSQL},
		}
	}

	return finding
}
