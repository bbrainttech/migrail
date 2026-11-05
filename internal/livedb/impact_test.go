package livedb

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/bbrainttech/migrail/internal/ir"
)

func scanLock(tables ...string) *ir.LockImpact {
	return &ir.LockImpact{Mode: "ACCESS EXCLUSIVE", Tables: tables, Blocks: []string{"SELECT", "INSERT", "UPDATE", "DELETE"}, Scan: true}
}

func TestAnnotateAdjustsSeverityByTableSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		severity ir.Severity
		table    Table
		want     ir.Severity
		note     string
	}{
		{name: "small error drops one level", severity: ir.SeverityError, table: Table{Rows: 50, TableBytes: 1 << 16}, want: ir.SeverityWarning, note: "small"},
		{name: "small warning drops one level", severity: ir.SeverityWarning, table: Table{Rows: 50, TableBytes: 1 << 16}, want: ir.SeverityNotice, note: "small"},
		{name: "few rows but many bytes stays", severity: ir.SeverityError, table: Table{Rows: 50, TableBytes: 1 << 30}, want: ir.SeverityError},
		{name: "unanalyzed table with no row estimate stays", severity: ir.SeverityError, table: Table{Rows: 0, TableBytes: 1 << 34}, want: ir.SeverityError},
		{name: "large by rows raises a warning", severity: ir.SeverityWarning, table: Table{Rows: 20_000_000, TableBytes: 1 << 30}, want: ir.SeverityError, note: "large"},
		{name: "large by bytes raises a warning", severity: ir.SeverityWarning, table: Table{Rows: 100_000, TableBytes: 11 << 30}, want: ir.SeverityError, note: "large"},
		{name: "medium table is unchanged", severity: ir.SeverityWarning, table: Table{Rows: 100_000, TableBytes: 1 << 26}, want: ir.SeverityWarning},
		{name: "a notice can't drop further", severity: ir.SeverityNotice, table: Table{Rows: 1, TableBytes: 1}, want: ir.SeverityNotice},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			findings := []ir.Finding{{Severity: tt.severity, Lock: scanLock("orders")}}
			Annotate(findings, map[string]Table{"orders": tt.table}, Thresholds{})

			if findings[0].Severity != tt.want {
				t.Errorf("Severity = %s, want %s", findings[0].Severity, tt.want)
			}

			if tt.note != "" && !strings.Contains(findings[0].Note, tt.note) {
				t.Errorf("Note = %q, want it to mention %q", findings[0].Note, tt.note)
			}
		})
	}
}

func TestAnnotateKeepsSeverityWhenWorkDoesNotGrowWithSize(t *testing.T) {
	t.Parallel()

	findings := []ir.Finding{{Severity: ir.SeverityError, Lock: &ir.LockImpact{Mode: "ACCESS EXCLUSIVE", Tables: []string{"orders"}}}}
	Annotate(findings, map[string]Table{"orders": {Rows: 1, TableBytes: 1}}, Thresholds{})

	if findings[0].Severity != ir.SeverityError {
		t.Errorf("Severity = %s, want error: a brief lock doesn't depend on table size", findings[0].Severity)
	}

	if findings[0].Impact == nil || findings[0].Impact.MaxDuration != 0 {
		t.Errorf("Impact = %+v, want sizes without a time estimate", findings[0].Impact)
	}
}

func TestAnnotateHonorsConfiguredThresholds(t *testing.T) {
	t.Parallel()

	findings := []ir.Finding{{Severity: ir.SeverityWarning, Lock: scanLock("orders")}}
	Annotate(findings, map[string]Table{"orders": {Rows: 500, TableBytes: 1 << 16}}, Thresholds{SmallRows: 100, LargeRows: 400})

	if findings[0].Severity != ir.SeverityError {
		t.Errorf("Severity = %s, want error for 500 rows with large_table_rows 400", findings[0].Severity)
	}
}

func TestAnnotateLeavesFindingsOnMissingTables(t *testing.T) {
	t.Parallel()

	findings := []ir.Finding{{Severity: ir.SeverityError, Lock: scanLock("orders")}}
	Annotate(findings, map[string]Table{}, Thresholds{})

	if findings[0].Severity != ir.SeverityError || findings[0].Impact != nil {
		t.Errorf("finding changed for a missing table: %+v", findings[0])
	}

	if !strings.Contains(findings[0].Note, `"orders" isn't in the connected database`) {
		t.Errorf("Note = %q, want it to say the table is missing", findings[0].Note)
	}
}

func TestAnnotateUsesTheLargestLockedTable(t *testing.T) {
	t.Parallel()

	findings := []ir.Finding{{Severity: ir.SeverityError, Lock: scanLock("users", "orders")}}
	Annotate(findings, map[string]Table{
		"users":  {Rows: 100_000, TableBytes: 1 << 24},
		"orders": {Rows: 5_000_000, TableBytes: 1 << 32},
	}, Thresholds{})

	if findings[0].Impact == nil || findings[0].Impact.Table != "orders" {
		t.Fatalf("Impact = %+v, want the larger table", findings[0].Impact)
	}
}

func TestEstimatesByOperation(t *testing.T) {
	t.Parallel()

	const gibibyte = 1 << 30

	table := Table{Rows: 1_000_000, TableBytes: gibibyte, IndexBytes: gibibyte}

	tests := []struct {
		name    string
		lock    ir.LockImpact
		minimum time.Duration
		maximum time.Duration
		blocked string
	}{
		{
			name:    "rewrite includes indexes",
			lock:    ir.LockImpact{Rewrite: true, Blocks: []string{"SELECT", "INSERT", "UPDATE", "DELETE"}},
			minimum: 14 * time.Second, maximum: 68 * time.Second, blocked: "reads and writes",
		},
		{
			name:    "index build reads the table only",
			lock:    ir.LockImpact{Scan: true, BuildsIndex: true, Blocks: []string{"INSERT", "UPDATE", "DELETE"}},
			minimum: 9 * time.Second, maximum: 41 * time.Second, blocked: "writes",
		},
		{
			name:    "validation that blocks nothing has no blocked text",
			lock:    ir.LockImpact{Scan: true},
			minimum: 2 * time.Second, maximum: 13 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			impact := impactOf(&tt.lock, "orders", table)

			if impact.MinDuration.Round(time.Second) != tt.minimum || impact.MaxDuration.Round(time.Second) != tt.maximum {
				t.Errorf("range = %v to %v, want %v to %v", impact.MinDuration, impact.MaxDuration, tt.minimum, tt.maximum)
			}

			if impact.Blocked != tt.blocked {
				t.Errorf("Blocked = %q, want %q", impact.Blocked, tt.blocked)
			}
		})
	}
}

func TestLockedTablesSkipsDuplicatesAndFindingsWithoutLocks(t *testing.T) {
	t.Parallel()

	got := LockedTables([]ir.Finding{
		{Lock: &ir.LockImpact{Tables: []string{"orders", "users"}}},
		{Lock: &ir.LockImpact{Tables: []string{"orders"}}},
		{},
		{Lock: &ir.LockImpact{Tables: []string{"ignored"}}, Suppressed: &ir.Suppression{Source: "inline"}},
	})

	if !slices.Equal(got, []string{"orders", "users"}) {
		t.Errorf("LockedTables() = %v", got)
	}
}
