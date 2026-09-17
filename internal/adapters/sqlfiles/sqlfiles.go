package sqlfiles

import (
	"path"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

type Adapter struct{}

func (Adapter) Name() string { return "sql" }

func (Adapter) Detect(dir adapters.Dir) bool {
	return strings.Contains(strings.ToLower(path.Base(dir.Path)), "migrat") && len(dir.SQLFiles()) > 0
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	files := []adapters.File{}

	for _, name := range dir.SQLFiles() {
		if strings.HasSuffix(name, ".down.sql") {
			continue
		}

		version, label, _ := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		files = append(files, adapters.File{Path: dir.Join(name), Version: version, Name: label})
	}

	adapters.SortByVersion(files)

	return files, nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeNonTransactional}
}
