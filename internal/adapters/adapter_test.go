package adapters

import (
	"io/fs"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestCompareVersions(t *testing.T) {
	t.Parallel()

	versions := []string{"10", "2", "1.10", "1.2", "1_1", "20260917101500", "001", "1.2.1"}
	slices.SortFunc(versions, CompareVersions)

	want := []string{"001", "1_1", "1.2", "1.2.1", "1.10", "2", "10", "20260917101500"}
	if !slices.Equal(versions, want) {
		t.Errorf("sorted = %v, want %v", versions, want)
	}
}

func TestKeepSectionsPreservesOffsets(t *testing.T) {
	t.Parallel()

	source := "-- header\r\n-- +up\nCREATE TABLE é (id int);\n-- +down\nDROP TABLE é;"
	masked := KeepSections(source, func(line string, inside bool) bool {
		switch {
		case strings.HasPrefix(line, "-- +up"):
			return true
		case strings.HasPrefix(line, "-- +down"):
			return false
		default:
			return inside
		}
	})

	if len(masked) != len(source) || strings.Count(masked, "\n") != strings.Count(source, "\n") {
		t.Fatalf("masking changed the layout: %q", masked)
	}

	if !strings.Contains(masked, "CREATE TABLE é (id int);") || strings.Contains(masked, "DROP") || strings.Contains(masked, "header") {
		t.Errorf("masked = %q", masked)
	}
}

func TestMatchFilesOrdersByVersion(t *testing.T) {
	t.Parallel()

	fsys := fstest.MapFS{
		"db/10_c.up.sql":  {},
		"db/2_b.up.sql":   {},
		"db/1_a.up.sql":   {},
		"db/1_a.down.sql": {},
		"db/notes.txt":    {},
	}

	entries, err := fs.ReadDir(fsys, "db")
	if err != nil {
		t.Fatal(err)
	}

	files := MatchFiles(Dir{FS: fsys, Path: "db", Entries: entries}, regexp.MustCompile(`^(\d+)_(.+)\.up\.sql$`))

	paths := []string{}
	for _, file := range files {
		paths = append(paths, file.Path)
	}

	if want := []string{"db/1_a.up.sql", "db/2_b.up.sql", "db/10_c.up.sql"}; !slices.Equal(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}
