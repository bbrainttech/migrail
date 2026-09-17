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

func DefaultConstraintName(table string, columns []string, suffix string) string {
	name := strings.Join(append([]string{table}, columns...), "_") + "_" + suffix
	if len(name) > maxIdentifierLength {
		name = name[:maxIdentifierLength]
	}

	return name
}

const maxIdentifierLength = 63
