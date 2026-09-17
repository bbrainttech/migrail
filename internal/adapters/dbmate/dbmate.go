package dbmate

import (
	"regexp"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	upMarker   = "migrate:up"
	downMarker = "migrate:down"
)

var versionedFile = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

type Adapter struct{}

func (Adapter) Name() string { return "dbmate" }

func (Adapter) Detect(dir adapters.Dir) bool {
	return dir.AnyFileContains("-- " + upMarker)
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	return adapters.MatchFiles(dir, versionedFile), nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	txMode := ir.TxModeTransactional

	sql := adapters.KeepSections(source, func(line string, inside bool) bool {
		switch {
		case adapters.HasDirective(line, upMarker):
			if strings.Contains(line, "transaction:false") {
				txMode = ir.TxModeNonTransactional
			}

			return true
		case adapters.HasDirective(line, downMarker):
			return false
		default:
			return inside
		}
	})

	return adapters.Extraction{SQL: sql, TxMode: txMode}
}
