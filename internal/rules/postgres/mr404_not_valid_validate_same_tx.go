package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr404.md
var mr404Docs string

type notValidValidateSameTx struct{}

func (notValidValidateSameTx) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR404",
		Slug:            "not-valid-validate-same-tx",
		Title:           "VALIDATE CONSTRAINT runs in the same transaction as ADD … NOT VALID",
		Category:        ir.CategoryTransaction,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr404Docs,
	}
}

func (notValidValidateSameTx) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	alter, ok := existingAlterTable(c, stmt)
	if !ok {
		return nil
	}

	addedHere := map[string]bool{}

	for _, cmd := range alter.cmds {
		if cmd.GetSubtype() == pg_query.AlterTableType_AT_AddConstraint && cmd.GetDef().GetConstraint().GetSkipValidation() {
			addedHere[pg.ConstraintName(alter.table, cmd.GetDef().GetConstraint())] = true
		}
	}

	findings := []ir.Finding{}

	for _, cmd := range alter.cmds {
		if cmd.GetSubtype() != pg_query.AlterTableType_AT_ValidateConstraint {
			continue
		}

		name := cmd.GetName()
		if !addedHere[name] && !c.NotValidConstraintInCurrentTransaction(alter.table, name) {
			continue
		}

		finding := ir.Finding{
			Title: fmt.Sprintf("Constraint %s is validated in the same transaction that added it", quoted(name)),
			Why: fmt.Sprintf(
				"The lock from ADD CONSTRAINT stays held until the transaction commits, so validation scans %s while writes stay blocked. "+
					"NOT VALID gives no benefit here.",
				alter.table,
			),
			Fix: &ir.Fix{
				Summary:   "Validate the constraint in a separate migration or transaction:",
				Framework: langSQL,
				Steps: []ir.FixStep{
					{Title: "Keep ADD CONSTRAINT … NOT VALID in this migration"},
					{Title: "Move VALIDATE CONSTRAINT to the next migration"},
				},
			},
		}

		if span, ok := alter.node.IdentifierSpan("CONSTRAINT", name); ok && !addedHere[name] {
			finding.Location.Span = span
		}

		findings = append(findings, finding)
	}

	return findings
}
