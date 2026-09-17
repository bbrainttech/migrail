package postgres

import (
	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

var postgresOnly = []ir.Dialect{ir.DialectPostgres}

func Rules() []analyze.Rule {
	return []analyze.Rule{
		createIndexNonConcurrent{},
		addForeignKeyValidating{},
		setNotNullScan{},
		alterColumnTypeRewrite{},
		renameColumn{},
	}
}
