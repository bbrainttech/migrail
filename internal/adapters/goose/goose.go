package goose

import (
	"regexp"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	upMarker      = "+goose Up"
	downMarker    = "+goose Down"
	noTransaction = "+goose NO TRANSACTION"
)

var versionedFile = regexp.MustCompile(`^(\d+)_(.+)\.sql$`)

type Adapter struct{}

func (Adapter) Name() string { return "goose" }

func (Adapter) Detect(dir adapters.Dir) bool {
	return dir.AnyFileContains(upMarker)
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	return adapters.MatchFiles(dir, versionedFile), nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	txMode := ir.TxModeTransactional

	sql := adapters.KeepSections(source, func(line string, inside bool) bool {
		switch {
		case adapters.HasDirective(line, noTransaction):
			txMode = ir.TxModeNonTransactional

			return inside
		case adapters.HasDirective(line, upMarker):
			return true
		case adapters.HasDirective(line, downMarker):
			return false
		default:
			return inside
		}
	})

	if !strings.Contains(source, upMarker) {
		sql = source
	}

	return adapters.Extraction{SQL: sql, TxMode: txMode}
}
