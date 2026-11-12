package livedb

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	megabyte = 1 << 20
	gigabyte = 1 << 30

	defaultSmallRows  = 10_000
	defaultLargeRows  = 10_000_000
	smallTableBytes   = 10 * megabyte
	largeTableBytes   = 10 * gigabyte
	blockedWrites     = "writes"
	blockedEverything = "reads and writes"
	statementDelete   = "DELETE"
	statementSelect   = "SELECT"
)

type Thresholds struct {
	SmallRows int64
	LargeRows int64
}

func (t Thresholds) withDefaults() Thresholds {
	if t.SmallRows == 0 {
		t.SmallRows = defaultSmallRows
	}

	if t.LargeRows == 0 {
		t.LargeRows = defaultLargeRows
	}

	return t
}

type throughput struct {
	slowBytesPerSecond int64
	fastBytesPerSecond int64
}

var (
	rewriteThroughput = throughput{slowBytesPerSecond: 30 * megabyte, fastBytesPerSecond: 150 * megabyte}
	scanThroughput    = throughput{slowBytesPerSecond: 80 * megabyte, fastBytesPerSecond: 600 * megabyte}
	buildThroughput   = throughput{slowBytesPerSecond: 25 * megabyte, fastBytesPerSecond: 120 * megabyte}
)

func (t throughput) durations(size int64) (fastest, slowest time.Duration) {
	fastest = time.Duration(float64(size) / float64(t.fastBytesPerSecond) * float64(time.Second))
	slowest = time.Duration(float64(size) / float64(t.slowBytesPerSecond) * float64(time.Second))

	return fastest, slowest
}

func LockedTables(findings []ir.Finding) []string {
	names := []string{}

	for _, finding := range findings {
		if finding.Lock == nil || finding.Suppressed != nil {
			continue
		}

		for _, table := range finding.Lock.Tables {
			if !slices.Contains(names, table) {
				names = append(names, table)
			}
		}
	}

	return names
}

func Annotate(findings []ir.Finding, tables map[string]Table, thresholds Thresholds) {
	thresholds = thresholds.withDefaults()

	for i := range findings {
		finding := &findings[i]
		if finding.Lock == nil {
			continue
		}

		name, table, ok := largestTable(finding.Lock.Tables, tables)
		if !ok {
			addNote(finding, quotedTables(finding.Lock.Tables)+" isn't in the connected database, so this finding has no size estimate.")

			continue
		}

		finding.Impact = impactOf(finding.Lock, name, table)
		adjustSeverity(finding, name, table, thresholds)
	}
}

func largestTable(names []string, tables map[string]Table) (string, Table, bool) {
	var (
		largestName string
		largest     Table
		found       bool
	)

	for _, name := range names {
		table, ok := tables[name]
		if ok && (!found || table.TotalBytes() > largest.TotalBytes()) {
			largestName, largest, found = name, table, true
		}
	}

	return largestName, largest, found
}

func impactOf(lock *ir.LockImpact, name string, table Table) *ir.Impact {
	impact := &ir.Impact{Table: name, Rows: table.Rows, Bytes: table.TotalBytes()}

	var fastest, slowest time.Duration

	switch {
	case lock.Rewrite:
		fastest, slowest = rewriteThroughput.durations(table.TotalBytes())
	case lock.Scan && lock.BuildsIndex:
		fastest, slowest = buildThroughput.durations(table.TableBytes)
	case lock.Scan:
		fastest, slowest = scanThroughput.durations(table.TableBytes)
	default:
		return impact
	}

	impact.MinDuration, impact.MaxDuration = fastest, slowest
	impact.Blocked = blockedBy(lock)

	return impact
}

func blockedBy(lock *ir.LockImpact) string {
	switch {
	case slices.Contains(lock.Blocks, statementSelect):
		return blockedEverything
	case slices.Contains(lock.Blocks, statementDelete):
		return blockedWrites
	default:
		return ""
	}
}

func adjustSeverity(finding *ir.Finding, name string, table Table, thresholds Thresholds) {
	if !finding.Lock.Rewrite && !finding.Lock.Scan {
		return
	}

	switch {
	case table.Rows < thresholds.SmallRows && table.TotalBytes() < smallTableBytes:
		lowered := lowerSeverity(finding.Severity)
		if lowered != finding.Severity {
			finding.Severity = lowered
			addNote(finding, fmt.Sprintf("Lowered because %q is small in the connected database. Check that production is the same size.", name))
		}
	case (table.Rows > thresholds.LargeRows || table.TotalBytes() > largeTableBytes) && finding.Severity == ir.SeverityWarning:
		finding.Severity = ir.SeverityError
		addNote(finding, fmt.Sprintf("Raised because %q is large in the connected database.", name))
	}
}

func lowerSeverity(severity ir.Severity) ir.Severity {
	switch severity {
	case ir.SeverityError:
		return ir.SeverityWarning
	case ir.SeverityWarning:
		return ir.SeverityNotice
	case ir.SeverityNotice, ir.SeverityOff:
		return severity
	default:
		return severity
	}
}

func addNote(finding *ir.Finding, note string) {
	if finding.Note != "" {
		finding.Note += " " + note

		return
	}

	finding.Note = note
}

func quotedTables(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, strconv.Quote(name))
	}

	return strings.Join(quoted, ", ")
}
