package gitx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/ir"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...) //nolint:gosec // test helper running git with fixed arguments
	cmd.Dir = dir

	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
	)

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestChangesSinceMergeBase(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()
	ctx := context.Background()

	git(t, dir, "init", "-q", "-b", "main")
	write(t, filepath.Join(dir, "db/0001_applied.sql"), "CREATE TABLE a (id int);\n")
	write(t, filepath.Join(dir, "db/0002_edited.sql"), "CREATE TABLE b (id int);\n")
	write(t, filepath.Join(dir, "db/0003_deleted.sql"), "CREATE TABLE c (id int);\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")

	git(t, dir, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(dir, "db/0002_edited.sql"), "CREATE TABLE b (id bigint);\n")
	write(t, filepath.Join(dir, "db/0004_committed.sql"), "CREATE INDEX i ON a (id);\n")
	git(t, dir, "rm", "-q", "db/0003_deleted.sql")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "feature")
	write(t, filepath.Join(dir, "db/0005_untracked.sql"), "ALTER TABLE a ADD COLUMN x int;\n")

	repo, ok := Open(ctx, filepath.Join(dir, "db"))
	if !ok {
		t.Fatal("Open() did not find the repository")
	}

	base, err := repo.ResolveBase(ctx, "", map[string]string{})
	if err != nil || base != "main" {
		t.Fatalf("ResolveBase() = %q, %v, want main", base, err)
	}

	changes, err := repo.Changes(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]ir.ChangeState{
		"db/0002_edited.sql":    ir.ChangeStateModified,
		"db/0004_committed.sql": ir.ChangeStateNew,
		"db/0005_untracked.sql": ir.ChangeStateNew,
	}

	if len(changes) != len(want) {
		t.Fatalf("changes = %v, want %v", changes, want)
	}

	for path, state := range want {
		if changes[path] != state {
			t.Errorf("changes[%s] = %q, want %q", path, changes[path], state)
		}
	}

	if _, err := repo.ResolveBase(ctx, "does-not-exist", nil); err == nil {
		t.Error("ResolveBase() with an unknown explicit base should fail")
	}

	mergeBase, err := repo.MergeBase(ctx, base)
	if err != nil {
		t.Fatal(err)
	}

	fallback, err := repo.diffSinceMergeBase(ctx, base)
	if err != nil || !strings.Contains(fallback, "db/0002_edited.sql") || mergeBase == "" {
		t.Errorf("diffSinceMergeBase() = %q, %v", fallback, err)
	}

	if relative, ok := repo.Relative(filepath.Join(dir, "db", "0001_applied.sql")); !ok || relative != "db/0001_applied.sql" {
		t.Errorf("Relative() = %q, %v", relative, ok)
	}
}

func TestOpenOutsideRepository(t *testing.T) {
	t.Parallel()

	if _, ok := Open(context.Background(), t.TempDir()); ok {
		t.Error("Open() found a repository in an empty temp directory")
	}
}
