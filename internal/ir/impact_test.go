package ir

import (
	"slices"
	"testing"
	"time"
)

func TestImpactParts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		impact Impact
		want   []string
	}{
		{
			name: "estimate with blocked operations",
			impact: Impact{
				Rows: 41_200_000, Bytes: 19_756_849_561,
				MinDuration: 3 * time.Minute, MaxDuration: 7 * time.Minute, Blocked: "writes",
			},
			want: []string{"41.2M rows", "18.4 GB", "est. 3–7 min of blocked writes"},
		},
		{name: "no estimate", impact: Impact{Rows: 950, Bytes: 8192}, want: []string{"950 rows", "8.0 KB"}},
		{
			name:   "rounds to one value",
			impact: Impact{Rows: 12_500, Bytes: 5 << 20, MinDuration: 100 * time.Second, MaxDuration: 110 * time.Second},
			want:   []string{"12.5K rows", "5.0 MB", "est. about 2 min"},
		},
		{
			name:   "very short",
			impact: Impact{Rows: 1, Bytes: 10, MinDuration: time.Millisecond, MaxDuration: 5 * time.Millisecond},
			want:   []string{"1 row", "10 B", "est. under 1 s"},
		},
		{
			name:   "hours",
			impact: Impact{Rows: 2_000_000_000, Bytes: 2 << 40, MinDuration: 2 * time.Hour, MaxDuration: 6 * time.Hour},
			want:   []string{"2.0B rows", "2.0 TB", "est. 2–6 h"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.impact.Parts("–"); !slices.Equal(got, tt.want) {
				t.Errorf("Parts() = %q, want %q", got, tt.want)
			}
		})
	}
}
