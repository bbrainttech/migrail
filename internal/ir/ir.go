package ir

type Dialect string

const DialectPostgres Dialect = "postgres"

type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

type TxMode string

const (
	TxModeUnknown          TxMode = "unknown"
	TxModeTransactional    TxMode = "transactional"
	TxModeNonTransactional TxMode = "non_transactional"
)

type Origin string

const (
	OriginRawSQL       Origin = "raw_sql"
	OriginFrameworkCLI Origin = "framework_cli"
	OriginCapture      Origin = "capture"
)

type ChangeState string

const (
	ChangeStateNew       ChangeState = "new"
	ChangeStateModified  ChangeState = "modified"
	ChangeStateUnchanged ChangeState = "unchanged"
)

type Migration struct {
	ID          string
	Version     string
	Name        string
	Framework   string
	SourcePath  string
	Source      string
	Direction   Direction
	TxMode      TxMode
	Statements  []*Statement
	ChangeState ChangeState
	Origin      Origin
}

type StmtKind string

const (
	StmtUnknown     StmtKind = "unknown"
	StmtParseError  StmtKind = "parse_error"
	StmtBegin       StmtKind = "begin"
	StmtCommit      StmtKind = "commit"
	StmtRollback    StmtKind = "rollback"
	StmtSet         StmtKind = "set"
	StmtReset       StmtKind = "reset"
	StmtCreateTable StmtKind = "create_table"
	StmtCreateIndex StmtKind = "create_index"
	StmtAlterTable  StmtKind = "alter_table"
	StmtRename      StmtKind = "rename"
	StmtDropTable   StmtKind = "drop_table"
	StmtDropIndex   StmtKind = "drop_index"
)

type Statement struct {
	Index      int
	SQL        string
	Node       any
	Kind       StmtKind
	Targets    []ObjectRef
	Columns    []ColumnRef
	Effects    []Effect
	Setting    *Setting
	Span       Span
	SourceMap  *SourceLoc
	InTx       bool
	ParseError *ParseError
}

type ObjectRef struct {
	Schema string
	Name   string
}

func (o ObjectRef) String() string {
	if o.Schema == "" {
		return o.Name
	}

	return o.Schema + "." + o.Name
}

type ColumnRef struct {
	Table ObjectRef
	Name  string
}

func (c ColumnRef) String() string {
	return c.Table.String() + "." + c.Name
}

type EffectKind string

const (
	EffectCreateTable        EffectKind = "create_table"
	EffectDropTable          EffectKind = "drop_table"
	EffectRenameTable        EffectKind = "rename_table"
	EffectAddNotNullCheck    EffectKind = "add_not_null_check"
	EffectValidateConstraint EffectKind = "validate_constraint"
	EffectDropConstraint     EffectKind = "drop_constraint"
)

type Effect struct {
	Kind       EffectKind
	Object     ObjectRef
	NewName    string
	Column     string
	Constraint string
	Validated  bool
}

type Setting struct {
	Name  string
	Value string
	Local bool
}

type ParseError struct {
	Message    string
	Span       Span
	CoversRest bool
}

type SourceLoc struct {
	Path string
	Span Span
}
