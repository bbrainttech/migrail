package discovery

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/adapters/atlasdir"
	"github.com/bbrainttech/migrail/internal/adapters/dbmate"
	"github.com/bbrainttech/migrail/internal/adapters/drizzle"
	"github.com/bbrainttech/migrail/internal/adapters/flyway"
	"github.com/bbrainttech/migrail/internal/adapters/golangmigrate"
	"github.com/bbrainttech/migrail/internal/adapters/goose"
	"github.com/bbrainttech/migrail/internal/adapters/prisma"
	"github.com/bbrainttech/migrail/internal/adapters/sqitch"
	"github.com/bbrainttech/migrail/internal/adapters/sqlfiles"
	"github.com/bbrainttech/migrail/internal/ir"
)

const ConfigFile = ".migrail.yaml"

var skippedDirs = []string{
	".git", ".hg", ".svn", "node_modules", "vendor", ".venv", "venv", "__pycache__",
	"dist", "build", "target", ".next", ".nuxt", ".terraform", ".idea", ".vscode",
}

type Project struct {
	Root    string
	Dir     string
	Adapter adapters.Adapter
}

type Parser func(sql string) ([]*ir.Statement, error)

func Adapters() []adapters.Adapter {
	return []adapters.Adapter{
		sqitch.Adapter{},
		atlasdir.Adapter{},
		prisma.Adapter{},
		drizzle.Adapter{},
		goose.Adapter{},
		dbmate.Adapter{},
		golangmigrate.Adapter{},
		flyway.Adapter{},
		sqlfiles.Adapter{},
	}
}

func AdapterNamed(name string) (adapters.Adapter, bool) {
	for _, adapter := range Adapters() {
		if adapter.Name() == name {
			return adapter, true
		}
	}

	return nil, false
}

func AdapterNames() []string {
	names := []string{}
	for _, adapter := range Adapters() {
		names = append(names, adapter.Name())
	}

	return names
}

func FindRoot(start string) string {
	dir := start

	for {
		for _, marker := range []string{ConfigFile, ".git"} {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}

		dir = parent
	}
}

func Detect(fsys fs.FS, dir string) (adapters.Adapter, bool) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, false
	}

	candidate := adapters.Dir{FS: fsys, Path: dir, Entries: entries}

	for _, adapter := range Adapters() {
		if adapter.Detect(candidate) {
			return adapter, true
		}
	}

	return nil, false
}

func Discover(ctx context.Context, root string) ([]Project, error) {
	fsys := os.DirFS(root)
	projects := []Project{}

	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrPermission) {
				return fs.SkipDir
			}

			return err
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if !entry.IsDir() {
			return nil
		}

		if name != "." && slices.Contains(skippedDirs, entry.Name()) {
			return fs.SkipDir
		}

		if adapter, ok := Detect(fsys, name); ok {
			projects = append(projects, Project{Root: root, Dir: name, Adapter: adapter})

			return fs.SkipDir
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover migrations under %s: %w", root, err)
	}

	return projects, nil
}

func (p Project) Files() ([]adapters.File, error) {
	fsys := os.DirFS(p.Root)

	entries, err := fs.ReadDir(fsys, p.Dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p.Dir, err)
	}

	files, err := p.Adapter.Files(adapters.Dir{FS: fsys, Path: p.Dir, Entries: entries})
	if err != nil {
		return nil, fmt.Errorf("list %s migrations in %s: %w", p.Adapter.Name(), p.Dir, err)
	}

	return files, nil
}

var templateExpression = regexp.MustCompile(`(?s)\{\{.*?\}\}`)

func MaskTemplates(sql string) string {
	if !strings.Contains(sql, "{{") {
		return sql
	}

	return templateExpression.ReplaceAllStringFunc(sql, func(expression string) string {
		var out strings.Builder

		filler := byte('_')

		for i := range len(expression) {
			if expression[i] == '\n' {
				out.WriteByte('\n')

				filler = ' '

				continue
			}

			out.WriteByte(filler)
		}

		return out.String()
	})
}

func Load(root string, adapter adapters.Adapter, file adapters.File, parse Parser) (*ir.Migration, error) {
	source, err := fs.ReadFile(os.DirFS(root), file.Path)
	if err != nil {
		return nil, fmt.Errorf("read migration %s: %w", file.Path, err)
	}

	extraction := adapter.Extract(string(source))

	statements, err := parse(MaskTemplates(extraction.SQL))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", file.Path, err)
	}

	name := file.Name
	if name == "" {
		name = path.Base(file.Path)
	}

	return &ir.Migration{
		ID:          adapter.Name() + ":" + file.Path,
		Version:     file.Version,
		Name:        name,
		Framework:   adapter.Name(),
		SourcePath:  file.Path,
		Source:      string(source),
		Direction:   ir.DirectionUp,
		TxMode:      extraction.TxMode,
		Statements:  statements,
		ChangeState: ir.ChangeStateNew,
		Origin:      ir.OriginRawSQL,
	}, nil
}
