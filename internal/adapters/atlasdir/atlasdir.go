package atlasdir

import (
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

type Adapter struct{}

func (Adapter) Name() string { return "atlas" }

func (Adapter) Detect(dir adapters.Dir) bool {
	return dir.Has("atlas.sum")
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	files := []adapters.File{}

	for _, name := range dir.SQLFiles() {
		version, label, _ := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		files = append(files, adapters.File{Path: dir.Join(name), Version: version, Name: label})
	}

	adapters.SortByVersion(files)

	return files, nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	txMode := ir.TxModeTransactional

	for line := range strings.SplitSeq(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "--") {
			break
		}

		if adapters.HasDirective(line, "atlas:txmode none") {
			txMode = ir.TxModeNonTransactional
		}
	}

	return adapters.Extraction{SQL: source, TxMode: txMode}
}
