package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bbrainttech/migrail/internal/adapters"
	"github.com/bbrainttech/migrail/internal/adapters/sqlfiles"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/discovery"
	"github.com/bbrainttech/migrail/internal/ir"
)

type loader struct {
	dialect *pg.Dialect
	cwd     string
	forced  adapters.Adapter
	seen    map[string]bool
}

func loadMigrations(ctx context.Context, dialect *pg.Dialect, args []string, opts checkOptions, stdin io.Reader) ([]*ir.Migration, error) {
	if slices.Contains(args, stdinArgument) {
		if len(args) > 1 {
			return nil, errors.New("- reads standard input and can't be combined with file paths")
		}

		return loadStdin(stdin)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return nil, internalError(fmt.Errorf("find working directory: %w", err))
	}

	l := loader{dialect: dialect, cwd: cwd, seen: map[string]bool{}}
	if opts.framework != "" {
		l.forced, _ = discovery.AdapterNamed(opts.framework)
	}

	if len(args) == 0 && len(opts.cfg.Projects) > 0 {
		return l.configuredProjects(ctx, opts)
	}

	if len(args) == 0 {
		root := opts.root
		if root == "" {
			root = discovery.FindRoot(cwd)
		}

		return l.directory(ctx, root)
	}

	migrations := []*ir.Migration{}

	for _, arg := range args {
		loaded, err := l.argument(ctx, arg)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, loaded...)
	}

	return migrations, nil
}

func loadStdin(stdin io.Reader) ([]*ir.Migration, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, internalError(fmt.Errorf("read standard input: %w", err))
	}

	return []*ir.Migration{{
		ID:          "sql:" + stdinPath,
		Name:        stdinPath,
		Framework:   sqlfiles.Adapter{}.Name(),
		SourcePath:  stdinPath,
		Source:      string(data),
		SQL:         string(data),
		Direction:   ir.DirectionUp,
		TxMode:      ir.TxModeNonTransactional,
		ChangeState: ir.ChangeStateNew,
		Origin:      ir.OriginRawSQL,
	}}, nil
}

func (l loader) argument(ctx context.Context, arg string) ([]*ir.Migration, error) {
	info, err := os.Stat(arg)
	if err != nil {
		return nil, fmt.Errorf("read migration %s: %w", filepath.ToSlash(filepath.Clean(arg)), err)
	}

	if info.IsDir() {
		return l.directory(ctx, arg)
	}

	dir := filepath.Dir(arg)
	adapter := l.forced

	if adapter == nil {
		detected, ok := discovery.Detect(os.DirFS(dir), ".")
		if !ok {
			detected = sqlfiles.Adapter{}
		}

		adapter = detected
	}

	migration, err := l.load(dir, adapter, adapters.File{Path: filepath.Base(arg)})
	if err != nil || migration == nil {
		return nil, err
	}

	return []*ir.Migration{migration}, nil
}

func (l loader) directory(ctx context.Context, root string) ([]*ir.Migration, error) {
	projects, err := l.projects(ctx, root)
	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		return nil, fmt.Errorf("no migrations found in %s: pass migration files or directories, or set --framework", l.display(root, "."))
	}

	migrations := []*ir.Migration{}

	for _, project := range projects {
		files, err := project.Files()
		if err != nil {
			return nil, internalError(err)
		}

		for _, file := range files {
			migration, err := l.load(project.Root, project.Adapter, file)
			if err != nil {
				return nil, err
			}

			if migration != nil {
				migrations = append(migrations, migration)
			}
		}
	}

	return migrations, nil
}

func (l loader) configuredProjects(ctx context.Context, opts checkOptions) ([]*ir.Migration, error) {
	migrations := []*ir.Migration{}

	for _, project := range opts.cfg.Projects {
		dir := filepath.Join(opts.root, filepath.FromSlash(project.Path), filepath.FromSlash(project.Migrations))
		projectLoader := l

		if project.Framework != "" {
			projectLoader.forced, _ = discovery.AdapterNamed(project.Framework)
		}

		loaded, err := projectLoader.directory(ctx, dir)
		if err != nil {
			return nil, err
		}

		migrations = append(migrations, loaded...)
	}

	return migrations, nil
}

func (l loader) projects(ctx context.Context, root string) ([]discovery.Project, error) {
	if l.forced != nil {
		return []discovery.Project{{Root: root, Dir: ".", Adapter: l.forced}}, nil
	}

	projects, err := discovery.Discover(ctx, root)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, err
		}

		return nil, internalError(err)
	}

	if len(projects) == 0 && hasSQLFiles(root) {
		projects = append(projects, discovery.Project{Root: root, Dir: ".", Adapter: sqlfiles.Adapter{}})
	}

	return projects, nil
}

func hasSQLFiles(dir string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, "*.sql"))

	return err == nil && len(matches) > 0
}

func (l loader) load(root string, adapter adapters.Adapter, file adapters.File) (*ir.Migration, error) {
	display := l.display(root, file.Path)
	if l.seen[display] {
		return nil, nil
	}

	l.seen[display] = true

	migration, err := discovery.Read(root, adapter, file)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read migration %s: %w", display, os.ErrNotExist)
		}

		return nil, internalError(err)
	}

	if migration.Name == "" || migration.Name == filepath.Base(file.Path) {
		migration.Name = strings.TrimSuffix(filepath.Base(file.Path), filepath.Ext(file.Path))
	}

	migration.SourcePath = display
	migration.ID = adapter.Name() + ":" + display

	return migration, nil
}

func (l loader) display(root, name string) string {
	absolute := filepath.Join(root, filepath.FromSlash(name))
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(l.cwd, absolute)
	}

	relative, err := filepath.Rel(l.cwd, absolute)
	if err != nil {
		return filepath.ToSlash(absolute)
	}

	return filepath.ToSlash(relative)
}
