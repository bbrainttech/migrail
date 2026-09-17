package golangmigrate

import (
	"regexp"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

var upFile = regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`)

type Adapter struct{}

func (Adapter) Name() string { return "golang-migrate" }

func (Adapter) Detect(dir adapters.Dir) bool {
	for _, name := range dir.SQLFiles() {
		if upFile.MatchString(name) && dir.Has(strings.TrimSuffix(name, ".up.sql")+".down.sql") {
			return true
		}
	}

	return false
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	return adapters.MatchFiles(dir, upFile), nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeNonTransactional}
}
