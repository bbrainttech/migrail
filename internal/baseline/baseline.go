package baseline

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	FileName       = ".migrail-baseline.json"
	SourceBaseline = "baseline"

	formatVersion = 1
	fileMode      = 0o644
	reason        = "recorded in the baseline"
)

type Entry struct {
	Fingerprint string `json:"fingerprint"`
	Rule        string `json:"rule"`
	Path        string `json:"path"`
	Title       string `json:"title"`
}

type File struct {
	Version  int     `json:"version"`
	Findings []Entry `json:"findings"`
}

func New(findings []ir.Finding) File {
	file := File{Version: formatVersion, Findings: []Entry{}}
	seen := map[string]struct{}{}

	for _, finding := range findings {
		if finding.Suppressed != nil {
			continue
		}

		if _, ok := seen[finding.Fingerprint]; ok {
			continue
		}

		seen[finding.Fingerprint] = struct{}{}

		file.Findings = append(file.Findings, Entry{
			Fingerprint: finding.Fingerprint,
			Rule:        finding.RuleID,
			Path:        finding.Location.Path,
			Title:       finding.Title,
		})
	}

	slices.SortFunc(file.Findings, func(a, b Entry) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Rule, b.Rule), cmp.Compare(a.Fingerprint, b.Fingerprint))
	})

	return file
}

func Load(path string) (File, bool, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, fs.ErrNotExist) {
		return File{}, false, nil
	}

	if err != nil {
		return File{}, false, fmt.Errorf("read baseline %s: %w", path, err)
	}

	var file File
	if err := json.Unmarshal(data, &file); err != nil {
		return File{}, false, fmt.Errorf("read baseline %s: %w. Run migrail baseline to create it again", path, err)
	}

	if file.Version != formatVersion {
		return File{}, false, fmt.Errorf("read baseline %s: version %d isn't supported. Run migrail baseline to create it again", path, file.Version)
	}

	return file, true, nil
}

func (f File) Save(path string) error {
	var buffer bytes.Buffer

	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(f); err != nil {
		return fmt.Errorf("encode baseline: %w", err)
	}

	if err := os.WriteFile(path, buffer.Bytes(), fileMode); err != nil {
		return fmt.Errorf("write baseline %s: %w", path, err)
	}

	return nil
}

func (f File) Apply(findings []ir.Finding) int {
	known := make(map[string]struct{}, len(f.Findings))
	for _, entry := range f.Findings {
		known[entry.Fingerprint] = struct{}{}
	}

	matched := 0

	for i := range findings {
		if findings[i].Suppressed != nil {
			continue
		}

		if _, ok := known[findings[i].Fingerprint]; ok {
			findings[i].Suppressed = &ir.Suppression{Reason: reason, Source: SourceBaseline}
			matched++
		}
	}

	return matched
}
