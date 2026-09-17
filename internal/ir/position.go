package ir

import (
	"sort"
	"unicode/utf8"
)

type Position struct {
	Offset int
	Line   int
	Column int
}

type Span struct {
	Start Position
	End   Position
}

type LineIndex struct {
	text       string
	lineStarts []int
}

func NewLineIndex(text string) *LineIndex {
	starts := []int{0}

	for i := range len(text) {
		if text[i] == '\n' {
			starts = append(starts, i+1)
		}
	}

	return &LineIndex{text: text, lineStarts: starts}
}

func (l *LineIndex) Position(offset int) Position {
	offset = max(0, min(offset, len(l.text)))
	line := sort.Search(len(l.lineStarts), func(i int) bool { return l.lineStarts[i] > offset }) - 1
	column := utf8.RuneCountInString(l.text[l.lineStarts[line]:offset]) + 1

	return Position{Offset: offset, Line: line + 1, Column: column}
}

func (l *LineIndex) Span(start, end int) Span {
	return Span{Start: l.Position(start), End: l.Position(end)}
}

func (l *LineIndex) RuneOffset(runeIndex int) int {
	offset := 0

	for range runeIndex {
		if offset >= len(l.text) {
			break
		}

		_, size := utf8.DecodeRuneInString(l.text[offset:])
		offset += size
	}

	return offset
}

func (l *LineIndex) LineCount() int {
	return len(l.lineStarts)
}

func (l *LineIndex) LineBounds(line int) (start, end int) {
	if line < 1 || line > len(l.lineStarts) {
		return 0, 0
	}

	start = l.lineStarts[line-1]
	end = len(l.text)

	if line < len(l.lineStarts) {
		end = l.lineStarts[line] - 1
	}

	if end > start && l.text[end-1] == '\r' {
		end--
	}

	return start, end
}
