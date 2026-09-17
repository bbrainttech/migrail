package drizzle

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

const journalPath = "meta/_journal.json"

type journal struct {
	Entries []struct {
		Idx int    `json:"idx"`
		Tag string `json:"tag"`
	} `json:"entries"`
}

type Adapter struct{}

func (Adapter) Name() string { return "drizzle" }

func (Adapter) Detect(dir adapters.Dir) bool {
	_, err := fs.Stat(dir.FS, dir.Join(journalPath))

	return err == nil
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	data, err := fs.ReadFile(dir.FS, dir.Join(journalPath))
	if err != nil {
		return nil, fmt.Errorf("read drizzle journal: %w", err)
	}

	var parsed journal
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parse drizzle journal %s: %w", dir.Join(journalPath), err)
	}

	files := make([]adapters.File, 0, len(parsed.Entries))

	for _, entry := range parsed.Entries {
		version, name, _ := strings.Cut(entry.Tag, "_")
		files = append(files, adapters.File{Path: path.Join(dir.Path, entry.Tag+".sql"), Version: version, Name: name})
	}

	return files, nil
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeTransactional}
}
