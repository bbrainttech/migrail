package postgres

import (
	"errors"
	"sort"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"
	"github.com/pganalyze/pg_query_go/v6/parser"

	"github.com/bbrainttech/migrail/internal/ir"
)

var (
	minVersion = ir.Version{Major: 12}
	maxVersion = ir.Version{Major: 18}
)

type Dialect struct{}

func New() *Dialect {
	return &Dialect{}
}

func (*Dialect) Name() ir.Dialect {
	return ir.DialectPostgres
}

func (*Dialect) DefaultVersion() ir.Version {
	return minVersion
}

func (*Dialect) SupportedVersions() (lowest, highest ir.Version) {
	return minVersion, maxVersion
}

func (*Dialect) Fingerprint(stmt *ir.Statement) string {
	if stmt.ParseError == nil {
		if fingerprint, err := pg_query.Fingerprint(stmt.SQL); err == nil {
			return fingerprint
		}
	}

	return strings.Join(strings.Fields(stmt.SQL), " ")
}

type source struct {
	text   string
	lines  *ir.LineIndex
	tokens []*pg_query.ScanToken
}

type piece struct {
	start int
	end   int
}

func (d *Dialect) Parse(sql string) ([]*ir.Statement, error) {
	src := &source{text: sql, lines: ir.NewLineIndex(sql)}

	scan, err := pg_query.Scan(sql)
	if err != nil {
		return []*ir.Statement{src.fileError(err)}, nil
	}

	src.tokens = significantTokens(scan.GetTokens())

	var statements []*ir.Statement

	tree, err := pg_query.Parse(sql)
	if err == nil {
		statements, err = src.statementsFromTree(tree, 0, len(sql))
	} else {
		statements, err = src.statementsFromPieces()
	}

	if err != nil {
		return nil, err
	}

	for i, stmt := range statements {
		stmt.Index = i
	}

	return statements, nil
}

func significantTokens(tokens []*pg_query.ScanToken) []*pg_query.ScanToken {
	significant := make([]*pg_query.ScanToken, 0, len(tokens))

	for _, token := range tokens {
		if token.GetToken() == pg_query.Token_SQL_COMMENT || token.GetToken() == pg_query.Token_C_COMMENT {
			continue
		}

		significant = append(significant, token)
	}

	return significant
}

func (s *source) statementsFromTree(tree *pg_query.ParseResult, base, limit int) ([]*ir.Statement, error) {
	statements := make([]*ir.Statement, 0, len(tree.GetStmts()))

	for _, raw := range tree.GetStmts() {
		start := base + int(raw.GetStmtLocation())
		end := limit

		if raw.GetStmtLen() > 0 {
			end = start + int(raw.GetStmtLen())
		}

		stmt, ok := s.newStatement(piece{start: start, end: end})
		if !ok {
			continue
		}

		node := &Node{Stmt: raw.GetStmt(), Base: base, source: s, first: stmt.Span.Start.Offset, last: stmt.Span.End.Offset}
		stmt.Node = node
		classify(stmt, raw.GetStmt())

		statements = append(statements, stmt)
	}

	return statements, nil
}

func (s *source) statementsFromPieces() ([]*ir.Statement, error) {
	statements := []*ir.Statement{}

	for _, p := range s.pieces() {
		text := s.text[p.start:p.end]

		tree, err := pg_query.Parse(text)
		if err != nil {
			stmt, ok := s.newStatement(p)
			if !ok {
				continue
			}

			stmt.Kind = ir.StmtParseError
			stmt.ParseError = s.parseError(p, err, stmt.Span)
			statements = append(statements, stmt)

			continue
		}

		parsed, err := s.statementsFromTree(tree, p.start, p.end)
		if err != nil {
			return nil, err
		}

		statements = append(statements, parsed...)
	}

	return statements, nil
}

func (s *source) pieces() []piece {
	pieces := []piece{}
	start := 0

	for _, token := range s.tokens {
		if token.GetToken() != pg_query.Token_ASCII_59 {
			continue
		}

		pieces = append(pieces, piece{start: start, end: int(token.GetStart())})
		start = int(token.GetEnd())
	}

	return append(pieces, piece{start: start, end: len(s.text)})
}

func (s *source) tokensIn(start, end int) []*pg_query.ScanToken {
	first := sort.Search(len(s.tokens), func(i int) bool { return int(s.tokens[i].GetStart()) >= start })
	last := first

	for last < len(s.tokens) && int(s.tokens[last].GetEnd()) <= end {
		last++
	}

	return s.tokens[first:last]
}

func (s *source) newStatement(p piece) (*ir.Statement, bool) {
	tokens := s.tokensIn(p.start, p.end)
	if len(tokens) > 0 && tokens[len(tokens)-1].GetToken() == pg_query.Token_ASCII_59 {
		tokens = tokens[:len(tokens)-1]
	}

	if len(tokens) == 0 {
		return nil, false
	}

	start := int(tokens[0].GetStart())
	end := int(tokens[len(tokens)-1].GetEnd())

	return &ir.Statement{
		SQL:  s.text[start:end],
		Kind: ir.StmtUnknown,
		Span: s.lines.Span(start, end),
	}, true
}

func (s *source) parseError(p piece, err error, fallback ir.Span) *ir.ParseError {
	var parseErr *parser.Error
	if !errors.As(err, &parseErr) || parseErr.Cursorpos <= 0 {
		return &ir.ParseError{Message: err.Error(), Span: fallback}
	}

	pieceLines := ir.NewLineIndex(s.text[p.start:p.end])
	offset := p.start + pieceLines.RuneOffset(parseErr.Cursorpos-1)

	return &ir.ParseError{Message: parseErr.Message, Span: s.tokenSpanAt(offset)}
}

func (s *source) tokenSpanAt(offset int) ir.Span {
	for _, token := range s.tokens {
		if int(token.GetStart()) <= offset && offset < int(token.GetEnd()) {
			return s.lines.Span(int(token.GetStart()), int(token.GetEnd()))
		}
	}

	return s.lines.Span(offset, min(offset+1, len(s.text)))
}

func (s *source) fileError(err error) *ir.Statement {
	span := s.lines.Span(0, len(s.text))
	message := err.Error()

	var parseErr *parser.Error
	if errors.As(err, &parseErr) {
		message = parseErr.Message
	}

	return &ir.Statement{
		SQL:        s.text,
		Kind:       ir.StmtParseError,
		Span:       span,
		ParseError: &ir.ParseError{Message: "could not tokenize SQL: " + message, Span: span},
	}
}
