package ir

type TokenClass int

const (
	TokenPlain TokenClass = iota
	TokenKeyword
	TokenString
	TokenComment
)

type Token struct {
	Start int
	End   int
	Class TokenClass
}
