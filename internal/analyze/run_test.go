package analyze

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/ir"
)

type fakeDialect struct{}

func (fakeDialect) Name() ir.Dialect { return ir.DialectPostgres }

func (fakeDialect) Fingerprint(stmt *ir.Statement) string { return stmt.SQL }

type recordingRule struct {
	meta  ir.RuleMeta
	check func(c *Context, stmt *ir.Statement) []ir.Finding
}

func (r *recordingRule) Meta() ir.RuleMeta { return r.meta }

func (r *recordingRule) Check(c *Context, stmt *ir.Statement) []ir.Finding {
	if r.check != nil {
		return r.check(c, stmt)
	}

	return nil
}

func statement(index int, kind ir.StmtKind, effects ...ir.Effect) *ir.Statement {
	return &ir.Statement{
		Index:   index,
		SQL:     string(kind),
		Kind:    kind,
		Effects: effects,
		Span:    ir.Span{Start: ir.Position{Offset: index * 10, Line: index + 1, Column: 1}},
	}
}

func TestTrackerState(t *testing.T) {
	t.Parallel()

	orders := ir.ObjectRef{Name: "orders"}
	shipments := ir.ObjectRef{Schema: "public", Name: "shipments"}

	migration := &ir.Migration{
		ID:         "m1",
		SourcePath: "m1.sql",
		TxMode:     ir.TxModeNonTransactional,
		Statements: []*ir.Statement{
			statement(0, ir.StmtUnknown),
			statement(1, ir.StmtBegin),
			{Index: 2, Kind: ir.StmtSet, Setting: &ir.Setting{Name: "lock_timeout", Value: "5s", Local: true}},
			statement(3, ir.StmtCreateTable, ir.Effect{Kind: ir.EffectCreateTable, Object: shipments}),
			statement(4, ir.StmtAlterTable, ir.Effect{Kind: ir.EffectAddNotNullCheck, Object: orders, Column: "status", Constraint: "c"}),
			statement(5, ir.StmtAlterTable, ir.Effect{Kind: ir.EffectValidateConstraint, Object: orders, Constraint: "c"}),
			statement(6, ir.StmtCommit),
			statement(7, ir.StmtUnknown),
		},
	}

	type snapshot struct {
		inTx        bool
		lockTimeout bool
		shipmentNew bool
		checked     bool
	}

	var snapshots []snapshot

	rule := &recordingRule{
		meta: ir.RuleMeta{ID: "MR000", DefaultEnabled: true},
		check: func(c *Context, _ *ir.Statement) []ir.Finding {
			_, hasTimeout := c.Setting("lock_timeout")
			snapshots = append(snapshots, snapshot{
				inTx:        c.InTransaction(),
				lockTimeout: hasTimeout,
				shipmentNew: c.IsNewTable(ir.ObjectRef{Name: "shipments"}),
				checked:     c.HasValidatedNotNullCheck(ir.ColumnRef{Table: ir.ObjectRef{Schema: "public", Name: "orders"}, Name: "status"}),
			})

			return nil
		},
	}

	if _, err := Run(context.Background(), fakeDialect{}, []*ir.Migration{migration}, []Rule{rule}, Options{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []snapshot{
		{},
		{},
		{inTx: true},
		{inTx: true, lockTimeout: true},
		{inTx: true, lockTimeout: true, shipmentNew: true},
		{inTx: true, lockTimeout: true, shipmentNew: true},
		{inTx: true, lockTimeout: true, shipmentNew: true, checked: true},
		{shipmentNew: true, checked: true},
	}

	if !slices.Equal(snapshots, want) {
		t.Errorf("snapshots:\n got  %+v\n want %+v", snapshots, want)
	}
}

func TestTransactionalMigrationAndSettingsReset(t *testing.T) {
	t.Parallel()

	first := &ir.Migration{ID: "m1", TxMode: ir.TxModeTransactional, Statements: []*ir.Statement{
		{Index: 0, Kind: ir.StmtSet, Setting: &ir.Setting{Name: "lock_timeout", Value: "5s"}},
		statement(1, ir.StmtUnknown),
	}}
	second := &ir.Migration{ID: "m2", TxMode: ir.TxModeUnknown, Statements: []*ir.Statement{statement(0, ir.StmtUnknown)}}

	var observed []string

	rule := &recordingRule{
		meta: ir.RuleMeta{ID: "MR000", DefaultEnabled: true},
		check: func(c *Context, stmt *ir.Statement) []ir.Finding {
			_, hasTimeout := c.Setting("lock_timeout")
			observed = append(observed, strings.Join([]string{c.Migration.ID, boolText(stmt.InTx), boolText(hasTimeout)}, ":"))

			return nil
		},
	}

	if _, err := Run(context.Background(), fakeDialect{}, []*ir.Migration{first, second}, []Rule{rule}, Options{}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	want := []string{"m1:yes:no", "m1:yes:yes", "m2:no:no"}
	if !slices.Equal(observed, want) {
		t.Errorf("observed = %v, want %v", observed, want)
	}
}

func boolText(value bool) string {
	if value {
		return "yes"
	}

	return "no"
}

func TestActiveRules(t *testing.T) {
	t.Parallel()

	v12 := ir.Version{Major: 12}
	v15 := ir.Version{Major: 15}
	rules := []Rule{
		&recordingRule{meta: ir.RuleMeta{ID: "MR101", Slug: "a", DefaultEnabled: true}},
		&recordingRule{meta: ir.RuleMeta{ID: "MR603", Slug: "off-by-default"}},
		&recordingRule{meta: ir.RuleMeta{ID: "MR111", Slug: "old-only", DefaultEnabled: true, MaxVersion: &v12}},
		&recordingRule{meta: ir.RuleMeta{ID: "MR701", Slug: "mysql", DefaultEnabled: true, Dialects: []ir.Dialect{"mysql"}}},
	}

	tests := []struct {
		name string
		opts Options
		want []string
	}{
		{name: "defaults on 12", opts: Options{DBVersion: v12}, want: []string{"MR101", "MR111"}},
		{name: "version filter", opts: Options{DBVersion: v15}, want: []string{"MR101"}},
		{name: "only enables disabled rule by slug", opts: Options{DBVersion: v15, Only: []string{"off-by-default"}}, want: []string{"MR603"}},
		{name: "skip", opts: Options{DBVersion: v12, Skip: []string{"MR111"}}, want: []string{"MR101"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ids := []string{}
			for _, rule := range ActiveRules(rules, ir.DialectPostgres, tt.opts) {
				ids = append(ids, rule.Meta().ID)
			}

			if !slices.Equal(ids, tt.want) {
				t.Errorf("ActiveRules() = %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestRunCompletesSortsAndDedupes(t *testing.T) {
	t.Parallel()

	migration := &ir.Migration{ID: "m1", SourcePath: "db/m1.sql", Statements: []*ir.Statement{statement(0, ir.StmtUnknown), statement(1, ir.StmtUnknown)}}

	rule := &recordingRule{
		meta: ir.RuleMeta{ID: "MR200", Slug: "later", DefaultSeverity: ir.SeverityWarning, DefaultEnabled: true},
		check: func(_ *Context, _ *ir.Statement) []ir.Finding {
			return []ir.Finding{{Title: "same"}, {Title: "same"}}
		},
	}
	panicking := &recordingRule{
		meta: ir.RuleMeta{ID: "MR100", DefaultEnabled: true},
		check: func(_ *Context, stmt *ir.Statement) []ir.Finding {
			if stmt.Index == 1 {
				panic("boom")
			}

			return []ir.Finding{{Title: "first", Severity: ir.SeverityError}}
		},
	}

	result, err := Run(context.Background(), fakeDialect{}, []*ir.Migration{migration}, []Rule{rule, panicking}, Options{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := []string{}
	for _, finding := range result.Findings {
		got = append(got, fmt.Sprintf("%s@%s:%d:%s", finding.RuleID, finding.Location.Path, finding.Location.Span.Start.Line, finding.Severity))

		if finding.Fingerprint == "" || finding.Confidence != ir.ConfidenceDefinite || finding.MigrationID != "m1" {
			t.Errorf("finding not completed: %+v", finding)
		}
	}

	want := []string{"MR100@db/m1.sql:1:error", "MR200@db/m1.sql:1:warning", "MR200@db/m1.sql:2:warning"}
	if !slices.Equal(got, want) {
		t.Errorf("findings = %v, want %v", got, want)
	}

	if len(result.RuleErrors) != 1 || result.RuleErrors[0].RuleID != "MR100" || !strings.Contains(result.RuleErrors[0].Message, "boom") {
		t.Errorf("RuleErrors = %+v", result.RuleErrors)
	}

	if result.Statements != 2 || result.RulesRun != 2 {
		t.Errorf("Statements = %d, RulesRun = %d", result.Statements, result.RulesRun)
	}
}

func TestRunStopsWhenCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, fakeDialect{}, []*ir.Migration{{ID: "m1"}}, nil, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}
