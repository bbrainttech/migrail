package postgres

import (
	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

var postgresOnly = []ir.Dialect{ir.DialectPostgres}

func Rules() []analyze.Rule {
	return []analyze.Rule{
		createIndexNonConcurrent{},
		dropIndexNonConcurrent{},
		addForeignKeyValidating{},
		addCheckValidating{},
		addUniqueConstraintBlocking{},
		setNotNullScan{},
		alterColumnTypeRewrite{},
		addColumnVolatileDefault{},
		vacuumFullOrCluster{},
		addColumnNotNullNoDefault{},
		dropColumnInUse{},
		renameColumn{},
		renameTable{},
		dropTableInUse{},
		concurrentInTransaction{},
		missingLockTimeout{},
		notValidValidateSameTx{},
		truncateTable{},
		dropTable{},
		deleteOrUpdateWithoutWhere{},
		dynamicSQL{},
	}
}
