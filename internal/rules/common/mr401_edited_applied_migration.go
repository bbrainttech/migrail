package common

import (
	_ "embed"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr401.md
var mr401Docs string

type editedAppliedMigration struct{}

func (editedAppliedMigration) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR401",
		Slug:            "edited-applied-migration",
		Title:           "A migration that is already on the base branch was edited",
		Category:        ir.CategoryTransaction,
		DefaultSeverity: ir.SeverityWarning,
		DefaultEnabled:  true,
		Docs:            mr401Docs,
	}
}

func (editedAppliedMigration) Check(*analyze.Context, *ir.Statement) []ir.Finding {
	return nil
}

func (editedAppliedMigration) CheckMigration(_ *analyze.Context, migration *ir.Migration) []ir.Finding {
	if migration.ChangeState != ir.ChangeStateModified {
		return nil
	}

	return []ir.Finding{{
		Title: "This migration is already on the base branch and was edited",
		Why: "Databases that already ran this migration won't run it again, so the edit never reaches them, " +
			"while new databases get the edited version. Environments drift apart without an error.",
		Fix: &ir.Fix{
			Summary:   "Keep applied migrations unchanged and add a new one:",
			Framework: "sql",
			Steps: []ir.FixStep{
				{Title: "Revert the changes to this file"},
				{Title: "Put the change in a new migration"},
			},
		},
	}}
}
