package baseline

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bbrainttech/migrail/internal/ir"
)

func finding(fingerprint, rule, path string) ir.Finding {
	return ir.Finding{Fingerprint: fingerprint, RuleID: rule, Title: "title " + rule, Location: ir.SourceLoc{Path: path}}
}

func TestNewSortsAndDeduplicates(t *testing.T) {
	t.Parallel()

	suppressed := finding("c", "MR102", "a.sql")
	suppressed.Suppressed = &ir.Suppression{Reason: "safe", Source: "inline"}

	file := New([]ir.Finding{finding("b", "MR201", "b.sql"), finding("a", "MR101", "a.sql"), finding("a", "MR101", "a.sql"), suppressed})

	if len(file.Findings) != 2 {
		t.Fatalf("Findings = %+v, want 2 entries without duplicates or suppressed findings", file.Findings)
	}

	if file.Findings[0].Path != "a.sql" || file.Findings[1].Path != "b.sql" {
		t.Errorf("Findings are not sorted by path: %+v", file.Findings)
	}
}

func TestSaveLoadApply(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), FileName)

	if err := New([]ir.Finding{finding("known", "MR101", "a.sql")}).Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	file, found, err := Load(path)
	if err != nil || !found {
		t.Fatalf("Load() = found %v, error %v", found, err)
	}

	alreadyIgnored := finding("known", "MR101", "a.sql")
	alreadyIgnored.Suppressed = &ir.Suppression{Reason: "inline reason", Source: "inline"}

	findings := []ir.Finding{finding("known", "MR101", "a.sql"), finding("new", "MR101", "b.sql"), alreadyIgnored}

	if matched := file.Apply(findings); matched != 1 {
		t.Errorf("Apply() = %d, want 1", matched)
	}

	if findings[0].Suppressed == nil || findings[0].Suppressed.Source != SourceBaseline {
		t.Errorf("known finding wasn't suppressed by the baseline: %+v", findings[0].Suppressed)
	}

	if findings[1].Suppressed != nil {
		t.Errorf("new finding was suppressed: %+v", findings[1].Suppressed)
	}

	if findings[2].Suppressed.Source != "inline" {
		t.Errorf("inline suppression was replaced: %+v", findings[2].Suppressed)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, found, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || found {
		t.Errorf("Load() = found %v, error %v; want not found and no error", found, err)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"not json":      "{",
		"wrong version": `{"version": 99, "findings": []}`,
	}

	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), FileName)
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			_, _, err := Load(path)
			if err == nil || !strings.Contains(err.Error(), "migrail baseline") {
				t.Errorf("Load() error = %v, want advice to run migrail baseline", err)
			}
		})
	}
}
