package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bbrainttech/migrail/internal/ir"
)

var ErrNoBase = errors.New("no base branch found")

type Repo struct {
	Root string
}

func Open(ctx context.Context, dir string) (Repo, bool) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repo{}, false
	}

	root := strings.TrimSpace(out)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}

	return Repo{Root: root}, true
}

func (r Repo) ResolveBase(ctx context.Context, explicit string, env map[string]string) (string, error) {
	candidates := []string{}

	switch {
	case explicit != "":
		candidates = append(candidates, explicit)
	case env["GITHUB_BASE_REF"] != "":
		candidates = append(candidates, "origin/"+env["GITHUB_BASE_REF"], env["GITHUB_BASE_REF"])
	case env["CI_MERGE_REQUEST_TARGET_BRANCH_NAME"] != "":
		name := env["CI_MERGE_REQUEST_TARGET_BRANCH_NAME"]
		candidates = append(candidates, "origin/"+name, name)
	}

	if explicit == "" {
		if head, err := run(ctx, r.Root, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
			candidates = append(candidates, strings.TrimSpace(head))
		}

		candidates = append(candidates, "origin/main", "origin/master", "main", "master")
	}

	for _, candidate := range candidates {
		if _, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", candidate+"^{commit}"); err == nil {
			return candidate, nil
		}
	}

	if explicit != "" {
		return "", fmt.Errorf("git base %q not found: %w", explicit, ErrNoBase)
	}

	return "", ErrNoBase
}

func (r Repo) MergeBase(ctx context.Context, base string) (string, error) {
	out, err := run(ctx, r.Root, "merge-base", base, "HEAD")
	if err != nil {
		return "", fmt.Errorf("find merge base with %s: %w", base, err)
	}

	return strings.TrimSpace(out), nil
}

func (r Repo) Changes(ctx context.Context, mergeBase string) (map[string]ir.ChangeState, error) {
	changes := map[string]ir.ChangeState{}

	diff, err := run(ctx, r.Root, "diff", "--name-status", "--no-renames", "-z", mergeBase, "--")
	if err != nil {
		return nil, fmt.Errorf("list changed files since %s: %w", mergeBase, err)
	}

	fields := strings.Split(strings.TrimSuffix(diff, "\x00"), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		switch fields[i] {
		case "A":
			changes[fields[i+1]] = ir.ChangeStateNew
		case "M", "T":
			changes[fields[i+1]] = ir.ChangeStateModified
		}
	}

	untracked, err := run(ctx, r.Root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}

	for name := range strings.SplitSeq(untracked, "\x00") {
		if name != "" {
			changes[name] = ir.ChangeStateNew
		}
	}

	return changes, nil
}

func (r Repo) Relative(absolute string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}

	if relative, err := filepath.Rel(r.Root, absolute); err == nil && !strings.HasPrefix(relative, "..") {
		return filepath.ToSlash(relative), true
	}

	prefix := r.Root + string(filepath.Separator)
	if len(absolute) <= len(prefix) || !strings.EqualFold(absolute[:len(prefix)], prefix) {
		return "", false
	}

	rootInfo, rootErr := os.Stat(r.Root)
	candidateInfo, candidateErr := os.Stat(absolute[:len(r.Root)])

	if rootErr != nil || candidateErr != nil || !os.SameFile(rootInfo, candidateInfo) {
		return "", false
	}

	return filepath.ToSlash(absolute[len(prefix):]), true
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // git is a fixed binary and the arguments come from migrail, not user input
	cmd.Dir = dir

	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}
