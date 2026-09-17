package rules

import (
	"cmp"
	"slices"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/rules/common"
	"github.com/bbrainttech/migrail/internal/rules/postgres"
)

func All() []analyze.Rule {
	all := slices.Concat(postgres.Rules(), common.Rules())

	slices.SortFunc(all, func(a, b analyze.Rule) int {
		return cmp.Compare(a.Meta().ID, b.Meta().ID)
	})

	return all
}
