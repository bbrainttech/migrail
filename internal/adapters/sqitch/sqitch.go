package sqitch

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/ir"
)

const planFile = "sqitch.plan"

type Adapter struct{}

func (Adapter) Name() string { return "sqitch" }

func (Adapter) Detect(dir adapters.Dir) bool {
	return dir.Has(planFile)
}

func (Adapter) Files(dir adapters.Dir) ([]adapters.File, error) {
	data, err := fs.ReadFile(dir.FS, dir.Join(planFile))
	if err != nil {
		return nil, fmt.Errorf("read sqitch plan: %w", err)
	}

	files := []adapters.File{}
	seen := map[string]bool{}

	for number, line := range strings.Split(string(data), "\n") {
		name := changeName(line)
		if name == "" || seen[name] {
			continue
		}

		seen[name] = true
		files = append(files, adapters.File{
			Path:    path.Join(dir.Path, "deploy", name+".sql"),
			Version: fmt.Sprintf("%06d", number+1),
			Name:    name,
		})
	}

	return files, nil
}

func changeName(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%") || strings.HasPrefix(trimmed, "@") || strings.HasPrefix(trimmed, "#") {
		return ""
	}

	fields := strings.Fields(trimmed)

	return strings.TrimPrefix(fields[0], "+")
}

func (Adapter) Extract(source string) adapters.Extraction {
	return adapters.Extraction{SQL: source, TxMode: ir.TxModeNonTransactional}
}
