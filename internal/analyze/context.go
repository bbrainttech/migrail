package analyze

import (
	"maps"

	"github.com/bbrainttech/migrail/internal/ir"
)

const defaultSchema = "public"

type Context struct {
	Dialect   ir.Dialect
	DBVersion ir.Version
	Migration *ir.Migration
	state     *tracker
}

func (c *Context) InTransaction() bool {
	return c.state.inTransaction(c.Migration)
}

func (c *Context) IsNewTable(table ir.ObjectRef) bool {
	_, ok := c.state.newTables[tableKey(table)]

	return ok
}

func (c *Context) IsNewIndex(index ir.ObjectRef) bool {
	_, ok := c.state.newIndexes[tableKey(index)]

	return ok
}

func (c *Context) NotValidConstraintInCurrentTransaction(table ir.ObjectRef, constraint string) bool {
	tx, ok := c.state.notValidConstraints[constraintKey(table, constraint)]

	return ok && tx != noTransaction && tx == c.state.transactionID(c.Migration)
}

func (c *Context) FirstInMigration(key string) bool {
	if _, seen := c.state.reported[key]; seen {
		return false
	}

	c.state.reported[key] = struct{}{}

	return true
}

func (c *Context) HasValidatedNotNullCheck(column ir.ColumnRef) bool {
	for _, check := range c.state.notNullChecks {
		if check.validated && check.table == tableKey(column.Table) && check.column == column.Name {
			return true
		}
	}

	return false
}

func (c *Context) Setting(name string) (string, bool) {
	setting, ok := c.state.settings[name]

	return setting.value, ok
}

type settingValue struct {
	value string
	local bool
}

type notNullCheck struct {
	table     string
	column    string
	validated bool
}

const noTransaction = 0

type tracker struct {
	explicitTx          bool
	txSequence          int
	explicitTxID        int
	migrationTxID       int
	newTables           map[string]struct{}
	newIndexes          map[string]struct{}
	notNullChecks       map[string]notNullCheck
	notValidConstraints map[string]int
	settings            map[string]settingValue
	reported            map[string]struct{}
	savepoint           *schemaState
}

type schemaState struct {
	newTables           map[string]struct{}
	newIndexes          map[string]struct{}
	notNullChecks       map[string]notNullCheck
	notValidConstraints map[string]int
}

func newTracker() *tracker {
	return &tracker{
		newTables:           map[string]struct{}{},
		newIndexes:          map[string]struct{}{},
		notNullChecks:       map[string]notNullCheck{},
		notValidConstraints: map[string]int{},
		settings:            map[string]settingValue{},
		reported:            map[string]struct{}{},
	}
}

func (t *tracker) transactionID(migration *ir.Migration) int {
	switch {
	case t.explicitTx:
		return t.explicitTxID
	case migration.TxMode == ir.TxModeTransactional:
		return t.migrationTxID
	default:
		return noTransaction
	}
}

func tableKey(table ir.ObjectRef) string {
	schema := table.Schema
	if schema == "" {
		schema = defaultSchema
	}

	return schema + "." + table.Name
}

func constraintKey(table ir.ObjectRef, constraint string) string {
	return tableKey(table) + "." + constraint
}

func (t *tracker) startMigration(migration *ir.Migration) {
	t.explicitTx = false
	t.settings = map[string]settingValue{}
	t.reported = map[string]struct{}{}
	t.migrationTxID = noTransaction

	if migration.TxMode == ir.TxModeTransactional {
		t.txSequence++
		t.migrationTxID = t.txSequence
	}
}

func (t *tracker) inTransaction(migration *ir.Migration) bool {
	return t.explicitTx || migration.TxMode == ir.TxModeTransactional
}

func (t *tracker) apply(migration *ir.Migration, stmt *ir.Statement) {
	switch stmt.Kind {
	case ir.StmtBegin:
		t.explicitTx = true
		t.txSequence++
		t.explicitTxID = t.txSequence
		t.savepoint = &schemaState{
			newTables:           maps.Clone(t.newTables),
			newIndexes:          maps.Clone(t.newIndexes),
			notNullChecks:       maps.Clone(t.notNullChecks),
			notValidConstraints: maps.Clone(t.notValidConstraints),
		}
	case ir.StmtCommit:
		t.endTransaction()
	case ir.StmtRollback:
		if t.savepoint != nil {
			t.newTables = t.savepoint.newTables
			t.newIndexes = t.savepoint.newIndexes
			t.notNullChecks = t.savepoint.notNullChecks
			t.notValidConstraints = t.savepoint.notValidConstraints
		}

		t.endTransaction()
	case ir.StmtSet:
		t.settings[stmt.Setting.Name] = settingValue{value: stmt.Setting.Value, local: stmt.Setting.Local}
	case ir.StmtReset:
		delete(t.settings, stmt.Setting.Name)
	default:
	}

	for _, effect := range stmt.Effects {
		t.applyEffect(migration, effect)
	}
}

func (t *tracker) endTransaction() {
	t.explicitTx = false
	t.savepoint = nil
	t.clearLocalSettings()
}

func (t *tracker) clearLocalSettings() {
	for name, setting := range t.settings {
		if setting.local {
			delete(t.settings, name)
		}
	}
}

func (t *tracker) applyEffect(migration *ir.Migration, effect ir.Effect) {
	switch effect.Kind {
	case ir.EffectCreateIndex:
		t.newIndexes[tableKey(effect.Object)] = struct{}{}
	case ir.EffectDropIndex:
		delete(t.newIndexes, tableKey(effect.Object))
	case ir.EffectAddConstraint:
		if !effect.Validated {
			t.notValidConstraints[constraintKey(effect.Object, effect.Constraint)] = t.transactionID(migration)
		}
	case ir.EffectCreateTable:
		t.newTables[tableKey(effect.Object)] = struct{}{}
	case ir.EffectDropTable:
		delete(t.newTables, tableKey(effect.Object))
		t.dropChecks(func(check notNullCheck) bool { return check.table == tableKey(effect.Object) })
	case ir.EffectDropConstraint:
		delete(t.notNullChecks, constraintKey(effect.Object, effect.Constraint))
		delete(t.notValidConstraints, constraintKey(effect.Object, effect.Constraint))
	case ir.EffectRenameTable:
		t.renameTable(effect)
	case ir.EffectAddNotNullCheck:
		t.notNullChecks[constraintKey(effect.Object, effect.Constraint)] = notNullCheck{
			table:     tableKey(effect.Object),
			column:    effect.Column,
			validated: effect.Validated,
		}
	case ir.EffectValidateConstraint:
		key := constraintKey(effect.Object, effect.Constraint)
		if check, ok := t.notNullChecks[key]; ok {
			check.validated = true
			t.notNullChecks[key] = check
		}
	}
}

func (t *tracker) dropChecks(matches func(notNullCheck) bool) {
	maps.DeleteFunc(t.notNullChecks, func(_ string, check notNullCheck) bool { return matches(check) })
}

func (t *tracker) renameTable(effect ir.Effect) {
	oldKey := tableKey(effect.Object)
	if _, ok := t.newTables[oldKey]; !ok {
		return
	}

	delete(t.newTables, oldKey)
	t.newTables[tableKey(ir.ObjectRef{Schema: effect.Object.Schema, Name: effect.NewName})] = struct{}{}
}
