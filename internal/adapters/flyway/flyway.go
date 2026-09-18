package flyway

import (
	"regexp"
	"slices"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

var (
	versionedFile  = regexp.MustCompile(`^V(\d+(?:[._]\d+)*)__(.+)\.sql$`)
	repeatableFile = regexp.MustCompile(`^R__(.+)\.sql$`)
)

type Adapter struct{}

func (Adapter) Name() string { return "flyway" }

func (Adapter) Detect(dir adapters.Dir) bool {
	if dir.Has("flyway.conf") || dir.Has("flyway.toml") {
		return true
	}

	return slices.ContainsFunc(dir.SQLFiles(), versionedFile.MatchString)
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	files := adapters.MatchFiles(dir, versionedFile)

	for _, name := range dir.SQLFiles() {
		if match := repeatableFile.FindStringSubmatch(name); match != nil {
			files = append(files, adapters.File{Path: dir.Join(name), Name: match[1]})
		}
	}

	return files, nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeByStatements}
}
