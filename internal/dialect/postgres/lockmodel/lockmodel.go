package lockmodel

import (
	"slices"

	"github.com/bbrainttech/migrail/internal/ir"
)

type Mode string

const (
	AccessShare          Mode = "ACCESS SHARE"
	RowShare             Mode = "ROW SHARE"
	RowExclusive         Mode = "ROW EXCLUSIVE"
	ShareUpdateExclusive Mode = "SHARE UPDATE EXCLUSIVE"
	Share                Mode = "SHARE"
	ShareRowExclusive    Mode = "SHARE ROW EXCLUSIVE"
	Exclusive            Mode = "EXCLUSIVE"
	AccessExclusive      Mode = "ACCESS EXCLUSIVE"
)

var conflicts = map[Mode][]Mode{
	AccessShare:          {AccessExclusive},
	RowShare:             {Exclusive, AccessExclusive},
	RowExclusive:         {Share, ShareRowExclusive, Exclusive, AccessExclusive},
	ShareUpdateExclusive: {ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	Share:                {RowExclusive, ShareUpdateExclusive, ShareRowExclusive, Exclusive, AccessExclusive},
	ShareRowExclusive:    {RowExclusive, ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	Exclusive:            {RowShare, RowExclusive, ShareUpdateExclusive, Share, ShareRowExclusive, Exclusive, AccessExclusive},
	AccessExclusive: {
		AccessShare, RowShare, RowExclusive, ShareUpdateExclusive,
		Share, ShareRowExclusive, Exclusive, AccessExclusive,
	},
}

const (
	statementSelect = "SELECT"
	statementInsert = "INSERT"
	statementUpdate = "UPDATE"
	statementDelete = "DELETE"
)

type dmlLock struct {
	statement string
	mode      Mode
}

var dmlLocks = []dmlLock{
	{statement: statementSelect, mode: AccessShare},
	{statement: statementInsert, mode: RowExclusive},
	{statement: statementUpdate, mode: RowExclusive},
	{statement: statementDelete, mode: RowExclusive},
}

func (m Mode) Conflicts(other Mode) bool {
	return slices.Contains(conflicts[m], other)
}

func (m Mode) BlockedStatements() []string {
	blocked := []string{}

	for _, lock := range dmlLocks {
		if m.Conflicts(lock.mode) {
			blocked = append(blocked, lock.statement)
		}
	}

	return blocked
}

type Operation string

const (
	CreateIndex             Operation = "create_index"
	CreateIndexConcurrently Operation = "create_index_concurrently"
	AddForeignKey           Operation = "add_foreign_key"
	ValidateConstraint      Operation = "validate_constraint"
	SetNotNull              Operation = "set_not_null"
	AlterColumnType         Operation = "alter_column_type"
	RenameColumn            Operation = "rename_column"
	DropIndex               Operation = "drop_index"
	AddCheck                Operation = "add_check"
	AddUnique               Operation = "add_unique"
	AddColumn               Operation = "add_column"
	AddColumnRewrite        Operation = "add_column_rewrite"
	DropColumn              Operation = "drop_column"
	RenameTable             Operation = "rename_table"
	DropTable               Operation = "drop_table"
	Truncate                Operation = "truncate"
	VacuumFull              Operation = "vacuum_full"
	Cluster                 Operation = "cluster"
)

type Entry struct {
	Mode    Mode
	Rewrite bool
	Scan    bool
}

var entries = map[Operation]Entry{
	CreateIndex:             {Mode: Share, Scan: true},
	CreateIndexConcurrently: {Mode: ShareUpdateExclusive, Scan: true},
	AddForeignKey:           {Mode: ShareRowExclusive, Scan: true},
	ValidateConstraint:      {Mode: ShareUpdateExclusive, Scan: true},
	SetNotNull:              {Mode: AccessExclusive, Scan: true},
	AlterColumnType:         {Mode: AccessExclusive, Rewrite: true},
	RenameColumn:            {Mode: AccessExclusive},
	DropIndex:               {Mode: AccessExclusive},
	AddCheck:                {Mode: AccessExclusive, Scan: true},
	AddUnique:               {Mode: AccessExclusive, Scan: true},
	AddColumn:               {Mode: AccessExclusive},
	AddColumnRewrite:        {Mode: AccessExclusive, Rewrite: true},
	DropColumn:              {Mode: AccessExclusive},
	RenameTable:             {Mode: AccessExclusive},
	DropTable:               {Mode: AccessExclusive},
	Truncate:                {Mode: AccessExclusive},
	VacuumFull:              {Mode: AccessExclusive, Rewrite: true},
	Cluster:                 {Mode: AccessExclusive, Rewrite: true},
}

func Lookup(op Operation) (Entry, bool) {
	entry, ok := entries[op]

	return entry, ok
}

func Impact(op Operation, tables ...ir.ObjectRef) *ir.LockImpact {
	entry, ok := Lookup(op)
	if !ok {
		return nil
	}

	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.String())
	}

	return &ir.LockImpact{
		Mode:    string(entry.Mode),
		Tables:  names,
		Blocks:  entry.Mode.BlockedStatements(),
		Rewrite: entry.Rewrite,
		Scan:    entry.Scan,
	}
}
