package postgres

import (
	_ "embed"
	"fmt"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr301.md
var mr301Docs string

type dropColumnInUse struct{}

func (dropColumnInUse) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR301",
		Slug:            "drop-column-in-use",
		Title:           "Dropping a column breaks code that still uses it",
		Category:        ir.CategoryCompatibility,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr301Docs,
	}
}

func (dropColumnInUse) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	alter, ok := existingAlterTable(c, stmt)
	if !ok {
		return nil
	}

	findings := []ir.Finding{}

	for _, cmd := range alter.cmds {
		if cmd.GetSubtype() != pg_query.AlterTableType_AT_DropColumn {
			continue
		}

		column := ir.ColumnRef{Table: alter.table, Name: cmd.GetName()}
		finding := ir.Finding{
			Confidence: ir.ConfidencePossible,
			Title:      fmt.Sprintf("Dropping column %s breaks code that still uses it", quoted(column.String())),
			Why: fmt.Sprintf(
				"During a rolling deploy, instances running the previous release still read or write %s, and ORMs that cache the column list fail too. "+
					"migrail doesn't scan application code yet, so check for remaining uses.",
				quoted(column.Name),
			),
			Lock: lockmodel.Impact(lockmodel.DropColumn, alter.table),
			Fix: &ir.Fix{
				Summary:   "Drop the column in two deploys:",
				Framework: langSQL,
				Steps: []ir.FixStep{
					{Title: fmt.Sprintf("Remove every read and write of %s from the application and deploy", column.Name)},
					{Title: "Drop the column in a later deploy"},
				},
			},
		}

		if span, ok := alter.node.IdentifierSpan("DROP", column.Name); ok {
			finding.Location.Span = span
		}

		findings = append(findings, finding)
	}

	return findings
}
