package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bbrainttech/migrail/internal/gitx"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/term"
)

type changeScope struct {
	Base        string
	ChangedOnly bool
}

func applyGitScope(
	ctx context.Context,
	migrations []*ir.Migration,
	explicitPaths bool,
	opts checkOptions,
	notice func(string),
) ([]*ir.Migration, changeScope, error) {
	checkEverything := explicitPaths || opts.all

	cwd, err := os.Getwd()
	if err != nil {
		return nil, changeScope{}, internalError(fmt.Errorf("find working directory: %w", err))
	}

	repo, ok := gitx.Open(ctx, cwd)
	if !ok {
		if !checkEverything {
			notice("this isn't a git repository, so migrail checks every migration.")
		}

		return migrations, changeScope{}, nil
	}

	base, err := repo.ResolveBase(ctx, opts.base, term.NewEnv(os.Environ()))
	if err != nil {
		if opts.base != "" {
			return nil, changeScope{}, fmt.Errorf("invalid --base: %w", err)
		}

		if !checkEverything {
			notice("no base branch found (tried origin/HEAD, main and master), so migrail checks every migration. Set --base to compare against a branch.")
		}

		return migrations, changeScope{}, nil
	}

	mergeBase, err := repo.MergeBase(ctx, base)
	if err == nil {
		var changes map[string]ir.ChangeState

		changes, err = repo.Changes(ctx, mergeBase)
		if err == nil {
			return filterChanged(migrations, repo, cwd, changes, checkEverything), changeScope{Base: base, ChangedOnly: !checkEverything}, nil
		}
	}

	if errors.Is(err, context.Canceled) {
		return nil, changeScope{}, err
	}

	notice(fmt.Sprintf("couldn't compare with %s, so migrail checks every migration. In CI, fetch the full history (for example fetch-depth: 0).", base))

	return migrations, changeScope{}, nil
}

func filterChanged(migrations []*ir.Migration, repo gitx.Repo, cwd string, changes map[string]ir.ChangeState, keepAll bool) []*ir.Migration {
	kept := make([]*ir.Migration, 0, len(migrations))

	for _, migration := range migrations {
		migration.ChangeState = ir.ChangeStateUnchanged

		if relative, ok := repo.Relative(filepath.Join(cwd, filepath.FromSlash(migration.SourcePath))); ok {
			if state, changed := changes[relative]; changed {
				migration.ChangeState = state
			}
		}

		if keepAll || migration.ChangeState != ir.ChangeStateUnchanged {
			kept = append(kept, migration)
		}
	}

	return kept
}
