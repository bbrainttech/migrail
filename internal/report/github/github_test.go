package github

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/ir"
)

func TestWriteEscapesWorkflowCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		finding ir.Finding
		want    string
	}{
		{
			name: "property separators in path and title",
			finding: ir.Finding{
				RuleID:   "MR101",
				Severity: ir.SeverityError,
				Title:    "Index on a,b: blocks writes",
				Why:      "100% of writes wait.\nEven small ones.",
				Location: ir.SourceLoc{Path: "db/1,2:x.sql", Span: span(3, 1, 3, 10)},
			},
			want: "::error file=db/1%2C2%3Ax.sql,line=3,endLine=3,col=1,endColumn=10,title=MR101 Index on a%2Cb%3A blocks writes::" +
				"100%25 of writes wait.%0AEven small ones.%0A%0ARun `migrail explain MR101` for details.\n",
		},
		{
			name: "multi-line span drops columns",
			finding: ir.Finding{
				RuleID:   "MR201",
				Severity: ir.SeverityNotice,
				Title:    "Rewrite",
				Location: ir.SourceLoc{Path: "a.sql", Span: span(1, 5, 4, 2)},
			},
			want: "::notice file=a.sql,line=1,endLine=4,title=MR201 Rewrite::Run `migrail explain MR201` for details.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var out bytes.Buffer
			if err := Write(&out, Input{Result: analyze.Result{Findings: []ir.Finding{tt.finding}}}); err != nil {
				t.Fatalf("write: %v", err)
			}

			first, _, _ := strings.Cut(out.String(), "migrail:")
			if first != tt.want {
				t.Errorf("got\n%s\nwant\n%s", first, tt.want)
			}
		})
	}
}

func TestWriteSkipsSuppressedFindings(t *testing.T) {
	t.Parallel()

	finding := ir.Finding{
		RuleID:     "MR101",
		Severity:   ir.SeverityError,
		Title:      "Index",
		Location:   ir.SourceLoc{Path: "a.sql", Span: span(1, 1, 1, 5)},
		Suppressed: &ir.Suppression{Reason: "empty table", Source: "inline"},
	}

	var out bytes.Buffer
	if err := Write(&out, Input{Result: analyze.Result{Findings: []ir.Finding{finding}}}); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got, want := out.String(), "migrail: 0 errors, 0 warnings, 0 notices\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func span(startLine, startColumn, endLine, endColumn int) ir.Span {
	return ir.Span{
		Start: ir.Position{Line: startLine, Column: startColumn},
		End:   ir.Position{Line: endLine, Column: endColumn},
	}
}
