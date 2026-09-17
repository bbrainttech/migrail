package postgres

import (
	"strings"
	"unicode"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/ir"
)

func (*Dialect) Highlight(source string) []ir.Token {
	scan, err := pg_query.Scan(source)
	if err != nil {
		return nil
	}

	tokens := make([]ir.Token, 0, len(scan.GetTokens()))

	for _, token := range scan.GetTokens() {
		start, end := int(token.GetStart()), int(token.GetEnd())
		if class := tokenClass(token, source[start:end]); class != ir.TokenPlain {
			tokens = append(tokens, ir.Token{Start: start, End: end, Class: class})
		}
	}

	return tokens
}

func tokenClass(token *pg_query.ScanToken, text string) ir.TokenClass {
	switch token.GetToken() {
	case pg_query.Token_SQL_COMMENT, pg_query.Token_C_COMMENT:
		return ir.TokenComment
	case pg_query.Token_SCONST, pg_query.Token_USCONST, pg_query.Token_BCONST, pg_query.Token_XCONST:
		return ir.TokenString
	default:
	}

	switch token.GetKeywordKind() {
	case pg_query.KeywordKind_RESERVED_KEYWORD:
		return ir.TokenKeyword
	case pg_query.KeywordKind_UNRESERVED_KEYWORD, pg_query.KeywordKind_COL_NAME_KEYWORD, pg_query.KeywordKind_TYPE_FUNC_NAME_KEYWORD:
		if isUpperWord(text) {
			return ir.TokenKeyword
		}
	default:
	}

	return ir.TokenPlain
}

func isUpperWord(text string) bool {
	return strings.IndexFunc(text, unicode.IsLower) < 0 && strings.IndexFunc(text, unicode.IsLetter) >= 0
}
