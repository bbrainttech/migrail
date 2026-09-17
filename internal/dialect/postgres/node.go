package postgres

import (
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/ir"
)

type Node struct {
	Stmt   *pg_query.Node
	Base   int
	source *source
	first  int
	last   int
}

func NodeOf(stmt *ir.Statement) (*Node, bool) {
	node, ok := stmt.Node.(*Node)
	if !ok || node == nil || node.Stmt == nil {
		return nil, false
	}

	return node, true
}

func (n *Node) tokens() []*pg_query.ScanToken {
	return n.source.tokensIn(n.first, n.last)
}

func (n *Node) tokenText(token *pg_query.ScanToken) string {
	return n.source.text[token.GetStart():token.GetEnd()]
}

func (n *Node) IdentifierSpan(afterKeyword, name string) (ir.Span, bool) {
	searching := afterKeyword == ""

	for _, token := range n.tokens() {
		text := n.tokenText(token)

		if !searching {
			searching = strings.EqualFold(text, afterKeyword)

			continue
		}

		if normalizeIdentifier(text) == name {
			return n.source.lines.Span(int(token.GetStart()), int(token.GetEnd())), true
		}
	}

	return ir.Span{}, false
}

func (n *Node) ClauseSpan(location int32) (ir.Span, bool) {
	start := n.Base + int(location)
	end := -1
	depth := 0

	for _, token := range n.tokens() {
		if int(token.GetStart()) < start {
			continue
		}

		switch token.GetToken() {
		case pg_query.Token_ASCII_40:
			depth++
		case pg_query.Token_ASCII_41:
			depth--
		case pg_query.Token_ASCII_44:
			if depth == 0 {
				return n.source.lines.Span(start, max(end, start)), end >= 0
			}
		default:
		}

		end = int(token.GetEnd())
	}

	return n.source.lines.Span(start, max(end, start)), end >= 0
}

func (n *Node) InsertAfterKeyword(keyword, insertion string) (string, bool) {
	for _, token := range n.tokens() {
		if !strings.EqualFold(n.tokenText(token), keyword) {
			continue
		}

		cut := int(token.GetEnd()) - n.first
		sql := n.source.text[n.first:n.last]

		return sql[:cut] + insertion + sql[cut:], true
	}

	return "", false
}

func normalizeIdentifier(text string) string {
	if len(text) >= 2 && strings.HasPrefix(text, `"`) && strings.HasSuffix(text, `"`) {
		return strings.ReplaceAll(text[1:len(text)-1], `""`, `"`)
	}

	return strings.ToLower(text)
}

func Deparse(stmt *pg_query.Node) (string, error) {
	tree := &pg_query.ParseResult{Stmts: []*pg_query.RawStmt{{Stmt: stmt}}}

	sql, err := pg_query.Deparse(tree)
	if err != nil {
		return "", fmt.Errorf("deparse statement: %w", err)
	}

	return sql, nil
}

func RelationRef(relation *pg_query.RangeVar) ir.ObjectRef {
	return ir.ObjectRef{Schema: relation.GetSchemaname(), Name: relation.GetRelname()}
}

func RangeVar(ref ir.ObjectRef) *pg_query.RangeVar {
	return &pg_query.RangeVar{Schemaname: ref.Schema, Relname: ref.Name, Inh: true, Relpersistence: "p"}
}

func TypeNameSQL(typeName *pg_query.TypeName) (string, error) {
	column := &pg_query.ColumnDef{Colname: "c", TypeName: typeName}
	stmt := &pg_query.CreateStmt{
		Relation:  &pg_query.RangeVar{Relname: "t", Inh: true, Relpersistence: "p"},
		TableElts: []*pg_query.Node{{Node: &pg_query.Node_ColumnDef{ColumnDef: column}}},
	}

	sql, err := Deparse(&pg_query.Node{Node: &pg_query.Node_CreateStmt{CreateStmt: stmt}})
	if err != nil {
		return "", err
	}

	inner := strings.TrimSuffix(strings.TrimPrefix(sql, "CREATE TABLE t (c "), ")")

	return inner, nil
}

func TypeBaseName(typeName *pg_query.TypeName) string {
	names := typeName.GetNames()
	if len(names) == 0 {
		return ""
	}

	return names[len(names)-1].GetString_().GetSval()
}

func StringValues(nodes []*pg_query.Node) []string {
	values := make([]string, 0, len(nodes))
	for _, node := range nodes {
		values = append(values, node.GetString_().GetSval())
	}

	return values
}

func DefaultConstraintName(table string, columns []string, label string) string {
	columnPart := strings.Join(columns, "_")
	overhead := len(label) + 1

	if columnPart != "" {
		overhead++
	}

	tableChars, columnChars := len(table), len(columnPart)
	for tableChars+columnChars > maxIdentifierLength-overhead {
		if tableChars > columnChars {
			tableChars--
		} else {
			columnChars--
		}
	}

	name := table[:tableChars]
	if columnPart != "" {
		name += "_" + columnPart[:columnChars]
	}

	return name + "_" + label
}

const maxIdentifierLength = 63
