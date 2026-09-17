package lockmodel

import (
	"slices"
	"testing"

	"github.com/bbrainttech/migrail/internal/ir"
)

func TestConflictMatrixIsSymmetric(t *testing.T) {
	t.Parallel()

	for mode := range conflicts {
		for other := range conflicts {
			if mode.Conflicts(other) != other.Conflicts(mode) {
				t.Errorf("%s vs %s conflict is not symmetric", mode, other)
			}
		}
	}
}

func TestBlockedStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mode Mode
		want []string
	}{
		{mode: AccessShare, want: []string{}},
		{mode: ShareUpdateExclusive, want: []string{}},
		{mode: Share, want: []string{"INSERT", "UPDATE", "DELETE"}},
		{mode: ShareRowExclusive, want: []string{"INSERT", "UPDATE", "DELETE"}},
		{mode: AccessExclusive, want: []string{"SELECT", "INSERT", "UPDATE", "DELETE"}},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			t.Parallel()

			if got := tt.mode.BlockedStatements(); !slices.Equal(got, tt.want) {
				t.Errorf("BlockedStatements() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestImpact(t *testing.T) {
	t.Parallel()

	impact := Impact(AddForeignKey, ir.ObjectRef{Name: "orders"}, ir.ObjectRef{Schema: "billing", Name: "users"})

	if impact.Mode != string(ShareRowExclusive) {
		t.Errorf("Mode = %q, want %q", impact.Mode, ShareRowExclusive)
	}

	if !slices.Equal(impact.Tables, []string{"orders", "billing.users"}) {
		t.Errorf("Tables = %v", impact.Tables)
	}

	if Impact(Operation("unknown")) != nil {
		t.Error("Impact() of an unknown operation should be nil")
	}
}
