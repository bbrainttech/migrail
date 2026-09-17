package adapters

import (
	"bufio"
	"cmp"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bbrainttech/migrail/internal/ir"
)

const headBytes = 8192

type Dir struct {
	FS      fs.FS
	Path    string
	Entries []fs.DirEntry
}

type File struct {
	Path    string
	Version string
	Name    string
}

type Extraction struct {
	SQL    string
	TxMode ir.TxMode
}

type Adapter interface {
	Name() string
	Detect(dir Dir) bool
	Files(dir Dir) ([]File, error)
	Extract(source string) Extraction
}

func (d Dir) Has(name string) bool {
	return slices.ContainsFunc(d.Entries, func(entry fs.DirEntry) bool { return entry.Name() == name })
}

func (d Dir) SQLFiles() []string {
	names := []string{}

	for _, entry := range d.Entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			names = append(names, entry.Name())
		}
	}

	return names
}

func (d Dir) Join(name string) string {
	if d.Path == "." || d.Path == "" {
		return name
	}

	return path.Join(d.Path, name)
}

func (d Dir) Head(name string) string {
	file, err := d.FS.Open(d.Join(name))
	if err != nil {
		return ""
	}

	buffer := make([]byte, headBytes)
	n, _ := file.Read(buffer)
	_ = file.Close()

	return string(buffer[:n])
}

func (d Dir) AnyFileContains(marker string) bool {
	for _, name := range d.SQLFiles() {
		if strings.Contains(d.Head(name), marker) {
			return true
		}
	}

	return false
}

func MatchFiles(dir Dir, pattern *regexp.Regexp) []File {
	files := []File{}

	for _, name := range dir.SQLFiles() {
		match := pattern.FindStringSubmatch(name)
		if match == nil {
			continue
		}

		files = append(files, File{Path: dir.Join(name), Version: match[1], Name: match[2]})
	}

	SortByVersion(files)

	return files
}

func SortByVersion(files []File) {
	slices.SortStableFunc(files, func(a, b File) int {
		return cmp.Or(CompareVersions(a.Version, b.Version), cmp.Compare(a.Path, b.Path))
	})
}

func CompareVersions(a, b string) int {
	partsA := strings.FieldsFunc(a, isVersionSeparator)
	partsB := strings.FieldsFunc(b, isVersionSeparator)

	for i := range min(len(partsA), len(partsB)) {
		numberA, errA := strconv.ParseUint(partsA[i], 10, 64)
		numberB, errB := strconv.ParseUint(partsB[i], 10, 64)

		if errA == nil && errB == nil {
			if numberA != numberB {
				return cmp.Compare(numberA, numberB)
			}

			continue
		}

		if order := cmp.Compare(partsA[i], partsB[i]); order != 0 {
			return order
		}
	}

	return cmp.Compare(len(partsA), len(partsB))
}

func isVersionSeparator(r rune) bool {
	return r == '.' || r == '_' || r == '-'
}

func KeepSections(source string, keep func(line string, inside bool) bool) string {
	var out strings.Builder

	inside := false
	scanner := bufio.NewScanner(strings.NewReader(source))
	scanner.Buffer(make([]byte, 0, 64*1024), len(source)+1)
	scanner.Split(scanLinesKeepingEndings)

	for scanner.Scan() {
		line := scanner.Text()
		inside = keep(line, inside)

		if inside {
			out.WriteString(line)

			continue
		}

		out.WriteString(blank(line))
	}

	return out.String()
}

func blank(line string) string {
	var out strings.Builder

	for i := range len(line) {
		if line[i] == '\n' || line[i] == '\r' {
			out.WriteByte(line[i])

			continue
		}

		out.WriteByte(' ')
	}

	return out.String()
}

func scanLinesKeepingEndings(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := slices.Index(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}

	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}

	return 0, nil, nil
}

func HasDirective(line, directive string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "--") {
		return false
	}

	return strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(trimmed, "--")), directive)
}
