package discovery

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
)

const projectsRoot = "../../testdata/projects"

type wantMigration struct {
	path   string
	txMode ir.TxMode
	kinds  string
}

func TestDiscoverAndLoadProjects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		project   string
		framework string
		dir       string
		want      []wantMigration
	}{
		{project: "golang-migrate", framework: "golang-migrate", dir: "db/migrations", want: []wantMigration{
			{path: "db/migrations/000001_create_orders.up.sql", txMode: ir.TxModeNonTransactional, kinds: "create_table"},
			{path: "db/migrations/000002_index_status.up.sql", txMode: ir.TxModeNonTransactional, kinds: "create_index"},
		}},
		{project: "goose", framework: "goose", dir: "migrations", want: []wantMigration{
			{path: "migrations/00001_create.sql", txMode: ir.TxModeTransactional, kinds: "unknown"},
			{path: "migrations/00002_add_index.sql", txMode: ir.TxModeTransactional, kinds: "create_index"},
			{path: "migrations/00003_no_tx.sql", txMode: ir.TxModeNonTransactional, kinds: "create_index"},
		}},
		{project: "atlas", framework: "atlas", dir: "migrations", want: []wantMigration{
			{path: "migrations/20260101000000_init.sql", txMode: ir.TxModeTransactional, kinds: "create_table"},
			{path: "migrations/20260102000000_rename.sql", txMode: ir.TxModeNonTransactional, kinds: "rename"},
		}},
		{project: "flyway", framework: "flyway", dir: "sql", want: []wantMigration{
			{path: "sql/V1__init.sql", txMode: ir.TxModeTransactional, kinds: "create_table"},
			{path: "sql/V1_2__add_email.sql", txMode: ir.TxModeTransactional, kinds: "alter_table"},
			{path: "sql/V1_10__later.sql", txMode: ir.TxModeTransactional, kinds: "alter_table"},
			{path: "sql/R__views.sql", txMode: ir.TxModeTransactional, kinds: "unknown"},
		}},
		{project: "prisma", framework: "prisma", dir: "prisma/migrations", want: []wantMigration{
			{path: "prisma/migrations/20260917101500_init/migration.sql", txMode: ir.TxModeNonTransactional, kinds: "create_table"},
			{path: "prisma/migrations/20260917102000_email_index/migration.sql", txMode: ir.TxModeNonTransactional, kinds: "create_index"},
		}},
		{project: "drizzle", framework: "drizzle", dir: "drizzle", want: []wantMigration{
			{path: "drizzle/0000_init.sql", txMode: ir.TxModeTransactional, kinds: "create_table"},
			{path: "drizzle/0001_status.sql", txMode: ir.TxModeTransactional, kinds: "alter_table,create_index"},
		}},
		{project: "dbmate", framework: "dbmate", dir: "db/migrations", want: []wantMigration{
			{path: "db/migrations/20260917101500_add_status.sql", txMode: ir.TxModeTransactional, kinds: "alter_table"},
			{path: "db/migrations/20260917102000_index.sql", txMode: ir.TxModeNonTransactional, kinds: "create_index"},
		}},
		{project: "sqitch", framework: "sqitch", dir: ".", want: []wantMigration{
			{path: "deploy/orders.sql", txMode: ir.TxModeNonTransactional, kinds: "begin,create_table,commit"},
			{path: "deploy/orders_status.sql", txMode: ir.TxModeNonTransactional, kinds: "begin,create_index,commit"},
		}},
		{project: "plain", framework: "sql", dir: "db/migrations", want: []wantMigration{
			{path: "db/migrations/001_create.sql", txMode: ir.TxModeNonTransactional, kinds: "create_table"},
			{path: "db/migrations/002_index.sql", txMode: ir.TxModeNonTransactional, kinds: "create_index"},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.project, func(t *testing.T) {
			t.Parallel()

			root := filepath.Join(projectsRoot, tt.project)

			projects, err := Discover(context.Background(), root)
			if err != nil {
				t.Fatalf("Discover() error = %v", err)
			}

			if len(projects) != 1 || projects[0].Adapter.Name() != tt.framework || projects[0].Dir != tt.dir {
				t.Fatalf("projects = %+v, want one %s project in %s", projects, tt.framework, tt.dir)
			}

			files, err := projects[0].Files()
			if err != nil {
				t.Fatalf("Files() error = %v", err)
			}

			got := []wantMigration{}

			for _, file := range files {
				migration, err := Load(root, projects[0].Adapter, file, pg.New().Parse)
				if err != nil {
					t.Fatalf("Load(%s) error = %v", file.Path, err)
				}

				kinds := []string{}
				for _, stmt := range migration.Statements {
					kinds = append(kinds, string(stmt.Kind))
				}

				got = append(got, wantMigration{path: migration.SourcePath, txMode: migration.TxMode, kinds: strings.Join(kinds, ",")})
			}

			if !slices.Equal(got, tt.want) {
				t.Errorf("migrations:\n got  %+v\n want %+v", got, tt.want)
			}
		})
	}
}

func TestMaskedSectionsKeepPositions(t *testing.T) {
	t.Parallel()

	root := filepath.Join(projectsRoot, "goose")
	adapter, _ := AdapterNamed("goose")

	files, err := Project{Root: root, Dir: "migrations", Adapter: adapter}.Files()
	if err != nil {
		t.Fatal(err)
	}

	migration, err := Load(root, adapter, files[1], pg.New().Parse)
	if err != nil {
		t.Fatal(err)
	}

	if len(migration.Statements) != 1 || migration.Statements[0].Span.Start.Line != 2 {
		t.Errorf("statements = %+v, want the CREATE INDEX on line 2", migration.Statements)
	}
}

func TestFindRoot(t *testing.T) {
	t.Parallel()

	start, err := filepath.Abs(filepath.Join(projectsRoot, "goose", "migrations"))
	if err != nil {
		t.Fatal(err)
	}

	root := FindRoot(start)
	if !strings.HasSuffix(filepath.ToSlash(root), "migrail") && filepath.Base(root) == "migrations" {
		t.Errorf("FindRoot(%s) = %s, want the repository root", start, root)
	}
}

func TestMaskTemplatesKeepsLayout(t *testing.T) {
	t.Parallel()

	source := "CREATE TABLE {{ index .Options \"Namespace\" }}.users (id int);\nCREATE INDEX i ON {{\n .Schema }}.users (id);\nSELECT '{}';\n"
	masked := MaskTemplates(source)

	if len(masked) != len(source) || strings.Count(masked, "\n") != strings.Count(source, "\n") || strings.Contains(masked, "{{") {
		t.Fatalf("MaskTemplates() = %q", masked)
	}

	statements, err := pg.New().Parse(masked)
	if err != nil || len(statements) != 3 || statements[0].ParseError != nil || statements[1].ParseError != nil {
		t.Fatalf("masked SQL doesn't parse: %+v, %v", statements, err)
	}

	if got := statements[0].Targets[0].String(); got != "users" {
		t.Errorf("template schema should be hidden, got %q", got)
	}

	if MaskTemplates("SELECT 1") != "SELECT 1" {
		t.Error("SQL without templates changed")
	}
}
