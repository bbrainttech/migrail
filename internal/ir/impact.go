package ir

import (
	"fmt"
	"strconv"
	"time"
)

type Impact struct {
	Table       string
	Rows        int64
	Bytes       int64
	MinDuration time.Duration
	MaxDuration time.Duration
	Blocked     string
}

func (i Impact) Parts(rangeDash string) []string {
	parts := []string{rowCount(i.Rows), compactBytes(i.Bytes)}

	if i.MaxDuration <= 0 {
		return parts
	}

	estimate := "est. " + durationRange(i.MinDuration, i.MaxDuration, rangeDash)
	if i.Blocked != "" {
		estimate += " of blocked " + i.Blocked
	}

	return append(parts, estimate)
}

func rowCount(rows int64) string {
	if rows == 1 {
		return "1 row"
	}

	return compactCount(rows) + " rows"
}

func compactCount(count int64) string {
	const (
		thousand = 1_000
		million  = 1_000_000
		billion  = 1_000_000_000
	)

	switch {
	case count >= billion:
		return strconv.FormatFloat(float64(count)/billion, 'f', 1, 64) + "B"
	case count >= million:
		return strconv.FormatFloat(float64(count)/million, 'f', 1, 64) + "M"
	case count >= thousand:
		return strconv.FormatFloat(float64(count)/thousand, 'f', 1, 64) + "K"
	default:
		return strconv.FormatInt(count, 10)
	}
}

func compactBytes(size int64) string {
	const (
		kib = 1 << 10
		mib = 1 << 20
		gib = 1 << 30
		tib = 1 << 40
	)

	switch {
	case size >= tib:
		return strconv.FormatFloat(float64(size)/tib, 'f', 1, 64) + " TB"
	case size >= gib:
		return strconv.FormatFloat(float64(size)/gib, 'f', 1, 64) + " GB"
	case size >= mib:
		return strconv.FormatFloat(float64(size)/mib, 'f', 1, 64) + " MB"
	case size >= kib:
		return strconv.FormatFloat(float64(size)/kib, 'f', 1, 64) + " KB"
	default:
		return strconv.FormatInt(size, 10) + " B"
	}
}

func durationRange(low, high time.Duration, dash string) string {
	if high < time.Second {
		return "under 1 s"
	}

	unit, name := time.Second, "s"

	switch {
	case high >= 2*time.Hour:
		unit, name = time.Hour, "h"
	case high >= time.Minute:
		unit, name = time.Minute, "min"
	}

	first := roundedUnits(low, unit)
	second := max(roundedUnits(high, unit), 1)

	if first >= second {
		return fmt.Sprintf("about %d %s", second, name)
	}

	return fmt.Sprintf("%d%s%d %s", max(first, 1), dash, second, name)
}

func roundedUnits(value, unit time.Duration) int64 {
	return int64((value + unit/2) / unit)
}
