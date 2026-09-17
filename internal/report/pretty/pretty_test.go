package pretty

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/golden"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/ui/term"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const (
	fixtures  = "../../../testdata/sql"
	goldenDir = "../../../testdata/golden/pretty"
)

type fixture struct {
	file string
	path string
}

func buildInput(t *testing.T, files []fixture) Input {
	t.Helper()

	dialect := pg.New()
	migrations := make([]*ir.Migration, 0, len(files))

	for _, f := range files {
		source, err := os.ReadFile(filepath.Join(fixtures, filepath.FromSlash(f.file)))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}

		statements, err := dialect.Parse(string(source))
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}

		migrations = append(migrations, &ir.Migration{
			ID:         "sql:" + f.path,
			SourcePath: f.path,
			Source:     string(source),
			Framework:  "sql",
			TxMode:     ir.TxModeNonTransactional,
			Statements: statements,
		})
	}

	version := ir.Version{Major: 16}

	result, err := analyze.Run(context.Background(), dialect, migrations, rules.All(), analyze.Options{DBVersion: version})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}

	return Input{
		ToolVersion: "0.1.0",
		Dialect:     dialect.Name(),
		DBVersion:   version,
		Migrations:  migrations,
		Result:      result,
		FailOn:      string(ir.SeverityError),
		Elapsed:     118 * time.Millisecond,
		Highlight:   dialect.Highlight,
	}
}

func TestScreensGolden(t *testing.T) {
	t.Parallel()

	findings := []fixture{
		{file: "MR101/bad/in_transaction.sql", path: "db/migrations/20260917101500_add_orders_index.sql"},
		{file: "MR104/bad/named.sql", path: "db/migrations/20260917101600_add_orders_user_fk.sql"},
		{file: "MR201/bad/to_varchar.sql", path: "db/migrations/20260917101700_widen_user_name.sql"},
		{file: "MR302/bad/basic.sql", path: "db/migrations/20260917102000_rename_user_email.sql"},
		{file: "MR101/bad/multiline_partial.sql", path: "db/migrations/20260917102100_index_user_email.sql"},
	}

	screens := []struct {
		name    string
		files   []fixture
		compact bool
	}{
		{name: "a_clean", files: []fixture{
			{file: "MR101/good/concurrently.sql", path: "db/migrations/0001_index.sql"},
			{file: "MR104/good/not_valid.sql", path: "db/migrations/0002_fk.sql"},
			{file: "MR302/good/new_table.sql", path: "db/migrations/0003_profiles.sql"},
		}},
		{name: "b_findings", files: findings},
		{name: "c_compact", files: findings, compact: true},
		{name: "e_parse_error", files: []fixture{
			{file: "MR901/bad/misspelled_keyword.sql", path: "db/migrations/20260917_x.sql"},
		}},
	}

	for _, screen := range screens {
		input := buildInput(t, screen.files)

		for _, profile := range golden.Profiles {
			for _, width := range golden.Widths {
				t.Run(screen.name+"/"+golden.Name(profile, width), func(t *testing.T) {
					t.Parallel()

					caps := term.Capabilities{Width: width}
					opts := Options{
						Theme:   theme.New(profile.Color, true, profile.Unicode),
						Width:   caps.ContentWidth(),
						Compact: screen.compact || caps.Compact(),
					}

					got := golden.Downsample(t, Render(input, opts), profile.Color)
					golden.Assert(t, filepath.Join(goldenDir, screen.name, golden.Name(profile, width)), got)
				})
			}
		}
	}
}

func TestHyperlinks(t *testing.T) {
	t.Parallel()

	input := buildInput(t, []fixture{{file: "MR302/bad/basic.sql", path: "db/rename.sql"}})
	opts := Options{
		Theme: theme.New(0, true, true),
		Width: 80,
		Link:  func(path string, _ int) string { return "file:///repo/" + path },
	}

	if got := Render(input, opts); !strings.Contains(got, "\x1b]8;;file:///repo/db/rename.sql") {
		t.Errorf("rendered output has no file hyperlink:\n%q", got)
	}
}
