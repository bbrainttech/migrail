package prisma

import (
	"io/fs"
	"path"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

const migrationFile = "migration.sql"

type Adapter struct{}

func (Adapter) Name() string { return "prisma" }

func (Adapter) Detect(dir adapters.Dir) bool {
	if dir.Has("migration_lock.toml") {
		return true
	}

	for _, entry := range dir.Entries {
		if entry.IsDir() && fileExists(dir.FS, path.Join(dir.Join(entry.Name()), migrationFile)) && path.Base(dir.Path) == "migrations" {
			return true
		}
	}

	return false
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	files := []adapters.File{}

	for _, entry := range dir.Entries {
		if !entry.IsDir() {
			continue
		}

		migration := path.Join(dir.Join(entry.Name()), migrationFile)
		if !fileExists(dir.FS, migration) {
			continue
		}

		version, name, _ := strings.Cut(entry.Name(), "_")
		files = append(files, adapters.File{Path: migration, Version: version, Name: name})
	}

	adapters.SortByVersion(files)

	return files, nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeNonTransactional}
}

func fileExists(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)

	return err == nil && !info.IsDir()
}
