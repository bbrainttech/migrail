package postgres

import (
	"slices"
	"testing"

	"github.com/bbrainttech/migrail/internal/ir"
)

type wantStatement struct {
	sql       string
	kind      ir.StmtKind
	startLine int
	startCol  int
	endLine   int
	endCol    int
}

func TestParseSplitsStatementsWithSpans(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		sql  string
		want []wantStatement
	}{
		{
			name: "comments and blank lines are skipped",
			sql:  "-- add status\n\nALTER TABLE orders ADD COLUMN status text;\n/* index */\nCREATE INDEX idx ON orders (status);\n",
			want: []wantStatement{
				{sql: "ALTER TABLE orders ADD COLUMN status text", kind: ir.StmtAlterTable, startLine: 3, startCol: 1, endLine: 3, endCol: 42},
				{sql: "CREATE INDEX idx ON orders (status)", kind: ir.StmtCreateIndex, startLine: 5, startCol: 1, endLine: 5, endCol: 36},
			},
		},
		{
			name: "multi-line statement without trailing semicolon",
			sql:  "BEGIN;\n  ALTER TABLE users\n    RENAME COLUMN email TO email_address",
			want: []wantStatement{
				{sql: "BEGIN", kind: ir.StmtBegin, startLine: 1, startCol: 1, endLine: 1, endCol: 6},
				{sql: "ALTER TABLE users\n    RENAME COLUMN email TO email_address", kind: ir.StmtRename, startLine: 2, startCol: 3, endLine: 3, endCol: 41},
			},
		},
		{
			name: "dollar-quoted body with semicolons stays one statement",
			sql:  "CREATE FUNCTION f() RETURNS void AS $$ BEGIN PERFORM 1; END; $$ LANGUAGE plpgsql;\nCOMMIT;",
			want: []wantStatement{
				{sql: "CREATE FUNCTION f() RETURNS void AS $$ BEGIN PERFORM 1; END; $$ LANGUAGE plpgsql", kind: ir.StmtUnknown, startLine: 1, startCol: 1, endLine: 1, endCol: 81},
				{sql: "COMMIT", kind: ir.StmtCommit, startLine: 2, startCol: 1, endLine: 2, endCol: 7},
			},
		},
		{
			name: "only comments",
			sql:  "-- nothing here\n/* still nothing */\n",
			want: []wantStatement{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			statements, err := New().Parse(tt.sql)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			assertStatements(t, statements, tt.want)
		})
	}
}

func assertStatements(t *testing.T, statements []*ir.Statement, want []wantStatement) {
	t.Helper()

	if len(statements) != len(want) {
		t.Fatalf("got %d statements, want %d", len(statements), len(want))
	}

	for i, stmt := range statements {
		got := wantStatement{
			sql:       stmt.SQL,
			kind:      stmt.Kind,
			startLine: stmt.Span.Start.Line,
			startCol:  stmt.Span.Start.Column,
			endLine:   stmt.Span.End.Line,
			endCol:    stmt.Span.End.Column,
		}

		if got != want[i] {
			t.Errorf("statement %d = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestParseRecoversAfterSyntaxError(t *testing.T) {
	t.Parallel()

	sql := "CREATE INDEX idx ON orders (status);\nALTER TABLE orders ADD COLUMN status txt text;\nDROP INDEX idx;\n"

	statements, err := New().Parse(sql)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	kinds := make([]ir.StmtKind, 0, len(statements))
	for i, stmt := range statements {
		kinds = append(kinds, stmt.Kind)

		if stmt.Index != i {
			t.Errorf("statement %d has Index %d", i, stmt.Index)
		}
	}

	wantKinds := []ir.StmtKind{ir.StmtCreateIndex, ir.StmtParseError, ir.StmtDropIndex}
	if !slices.Equal(kinds, wantKinds) {
		t.Fatalf("kinds = %v, want %v", kinds, wantKinds)
	}

	parseErr := statements[1].ParseError
	if parseErr.Message != `syntax error at or near "text"` {
		t.Errorf("message = %q", parseErr.Message)
	}

	if parseErr.Span.Start.Line != 2 || parseErr.Span.Start.Column != 42 || parseErr.Span.End.Column != 46 {
		t.Errorf("error span = %+v", parseErr.Span)
	}
}

func TestParseUnterminatedString(t *testing.T) {
	t.Parallel()

	statements, err := New().Parse("SELECT 'unterminated")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(statements) != 1 || statements[0].ParseError == nil {
		t.Fatalf("want a single parse error statement, got %+v", statements)
	}
}

func TestClassify(t *testing.T) {
	t.Parallel()

	statements, err := New().Parse(`
SET LOCAL lock_timeout = '5s';
RESET statement_timeout;
CREATE TABLE billing.invoices (id bigint);
ALTER TABLE orders ADD CONSTRAINT orders_status_nn CHECK (status IS NOT NULL) NOT VALID;
ALTER TABLE orders ADD CHECK (total IS NOT NULL), VALIDATE CONSTRAINT orders_status_nn;
ALTER TABLE "Users" RENAME TO people;
DROP TABLE a, public.b;
`)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	wantSettings := []ir.Setting{{Name: "lock_timeout", Value: "5s", Local: true}, {Name: "statement_timeout"}}
	for i, want := range wantSettings {
		if *statements[i].Setting != want {
			t.Errorf("setting %d = %+v, want %+v", i, *statements[i].Setting, want)
		}
	}

	orders := ir.ObjectRef{Name: "orders"}
	wantEffects := [][]ir.Effect{
		{{Kind: ir.EffectCreateTable, Object: ir.ObjectRef{Schema: "billing", Name: "invoices"}}},
		{{Kind: ir.EffectAddNotNullCheck, Object: orders, Column: "status", Constraint: "orders_status_nn"}},
		{
			{Kind: ir.EffectAddNotNullCheck, Object: orders, Column: "total", Constraint: "orders_total_check", Validated: true},
			{Kind: ir.EffectValidateConstraint, Object: orders, Constraint: "orders_status_nn"},
		},
		{{Kind: ir.EffectRenameTable, Object: ir.ObjectRef{Name: "Users"}, NewName: "people"}},
		{
			{Kind: ir.EffectDropTable, Object: ir.ObjectRef{Name: "a"}},
			{Kind: ir.EffectDropTable, Object: ir.ObjectRef{Schema: "public", Name: "b"}},
		},
	}

	for i, want := range wantEffects {
		if got := statements[i+2].Effects; !slices.Equal(got, want) {
			t.Errorf("statement %d effects = %+v, want %+v", i+2, got, want)
		}
	}
}

func TestNodeHelpers(t *testing.T) {
	t.Parallel()

	statements, err := New().Parse("SELECT 1;\n  create index idx on orders (status);\nALTER TABLE users RENAME COLUMN \"Email\" TO email;")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	index, _ := NodeOf(statements[1])

	fixed, ok := index.InsertAfterKeyword("INDEX", " CONCURRENTLY")
	if !ok || fixed != "create index CONCURRENTLY idx on orders (status)" {
		t.Errorf("InsertAfterKeyword() = %q, %v", fixed, ok)
	}

	rename, _ := NodeOf(statements[2])

	span, ok := rename.IdentifierSpan("RENAME", "Email")
	if !ok || span.Start.Line != 3 || span.Start.Column != 33 || span.End.Column != 40 {
		t.Errorf("IdentifierSpan() = %+v, %v", span, ok)
	}

	if _, ok := rename.IdentifierSpan("RENAME", "users"); ok {
		t.Error("IdentifierSpan() matched a token before the keyword")
	}

	if _, ok := NodeOf(&ir.Statement{}); ok {
		t.Error("NodeOf() on a statement without a node should fail")
	}
}

func TestTypeNameSQL(t *testing.T) {
	t.Parallel()

	statements, err := New().Parse("ALTER TABLE t ALTER COLUMN a TYPE numeric(10, 2), ALTER COLUMN b TYPE bigint")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	node, _ := NodeOf(statements[0])
	want := []string{"numeric(10, 2)", "bigint"}

	for i, cmd := range node.Stmt.GetAlterTableStmt().GetCmds() {
		got, err := TypeNameSQL(cmd.GetAlterTableCmd().GetDef().GetColumnDef().GetTypeName())
		if err != nil || got != want[i] {
			t.Errorf("TypeNameSQL() = %q, %v, want %q", got, err, want[i])
		}
	}
}
