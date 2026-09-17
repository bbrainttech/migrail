package postgres

import (
	_ "embed"
	"fmt"
	"slices"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr202.md
var mr202Docs string

var (
	volatileFunctions = []string{
		"random", "gen_random_uuid", "uuid_generate_v1", "uuid_generate_v1mc", "uuid_generate_v4",
		"uuidv4", "uuidv7", "clock_timestamp", "timeofday", "nextval", "gen_random_bytes", "setseed",
	}
	nonVolatileFunctions = []string{
		"now", "transaction_timestamp", "statement_timestamp", "current_setting", "lower", "upper",
		"concat", "to_jsonb", "jsonb_build_object", "jsonb_build_array", "json_build_object",
		"json_build_array", "make_interval", "date_trunc", "timezone", "to_timestamp", "array_fill",
	}
)

type addColumnVolatileDefault struct{}

func (addColumnVolatileDefault) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR202",
		Slug:            "add-column-volatile-default",
		Title:           "Adding a column with a volatile default rewrites the table",
		Category:        ir.CategoryRewrite,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr202Docs,
	}
}

func (addColumnVolatileDefault) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
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

		for _, def := range columnConstraints(column, pg_query.ConstrType_CONSTR_DEFAULT) {
			finding, found := volatileDefaultFinding(alter.table, column, def)
			if !found {
				continue
			}

			alter.spanAt(&finding, column.GetLocation())
			findings = append(findings, finding)
		}
	}

	return findings
}

func volatileDefaultFinding(table ir.ObjectRef, column *pg_query.ColumnDef, def *pg_query.Constraint) (ir.Finding, bool) {
	volatile, unknown := classifyFunctions(pg.FuncCalls(def.GetRawExpr()))
	if volatile == "" && unknown == "" {
		return ir.Finding{}, false
	}

	ref := ir.ColumnRef{Table: table, Name: column.GetColname()}
	finding := ir.Finding{
		Severity:   ir.SeverityError,
		Confidence: ir.ConfidenceDefinite,
		Title:      fmt.Sprintf("Adding %s with a volatile default rewrites the table", quoted(ref.String())),
		Why: fmt.Sprintf(
			"%s() returns a different value for every row, so Postgres computes it for each existing row and rewrites all of %s under an ACCESS EXCLUSIVE lock.",
			volatile, table,
		),
		Lock: lockmodel.Impact(lockmodel.AddColumnRewrite, table),
		Fix: &ir.Fix{
			Summary:   "Add the column without the default, set the default for new rows, then backfill:",
			Framework: langSQL,
			Steps:     volatileDefaultSteps(table, column, def),
		},
	}

	if volatile == "" {
		finding.Severity = ir.SeverityWarning
		finding.Confidence = ir.ConfidencePossible
		finding.Title = fmt.Sprintf("Adding %s with a function default may rewrite the table", quoted(ref.String()))
		finding.Why = fmt.Sprintf(
			"If %s() is volatile, Postgres computes it for each existing row and rewrites all of %s under an ACCESS EXCLUSIVE lock. "+
				"migrail can't tell whether this function is volatile.",
			unknown, table,
		)
	}

	return finding, true
}

func classifyFunctions(calls []*pg_query.FuncCall) (volatile, unknown string) {
	for _, call := range calls {
		name := strings.ToLower(pg.FuncName(call))

		switch {
		case slices.Contains(volatileFunctions, name):
			return name, ""
		case !slices.Contains(nonVolatileFunctions, name) && unknown == "":
			unknown = name
		}
	}

	return "", unknown
}

func volatileDefaultSteps(table ir.ObjectRef, column *pg_query.ColumnDef, def *pg_query.Constraint) []ir.FixStep {
	addSQL, addErr := alterTableSQL(table, addColumnCmd(withoutConstraints(column, pg_query.ConstrType_CONSTR_DEFAULT, pg_query.ConstrType_CONSTR_NOTNULL)))

	setDefault := &pg_query.AlterTableCmd{
		Subtype:  pg_query.AlterTableType_AT_ColumnDefault,
		Name:     column.GetColname(),
		Def:      def.GetRawExpr(),
		Behavior: pg_query.DropBehavior_DROP_RESTRICT,
	}
	defaultSQL, defaultErr := alterTableSQL(table, setDefault)

	if addErr != nil || defaultErr != nil {
		return []ir.FixStep{{Title: "Add the column without a default, set the default, then backfill existing rows in batches"}}
	}

	return []ir.FixStep{
		{Title: "Add the column without a default", Lang: langSQL, Code: addSQL},
		{Title: "Set the default for new rows", Lang: langSQL, Code: defaultSQL},
		{Title: fmt.Sprintf("Backfill %s for existing rows in batches", column.GetColname())},
	}
}
