package analyze

import "github.com/bbrainttech/migrail/internal/ir"

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

type tracker struct {
	explicitTx    bool
	newTables     map[string]struct{}
	notNullChecks map[string]notNullCheck
	settings      map[string]settingValue
}

func newTracker() *tracker {
	return &tracker{
		newTables:     map[string]struct{}{},
		notNullChecks: map[string]notNullCheck{},
		settings:      map[string]settingValue{},
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

func (t *tracker) startMigration() {
	t.explicitTx = false
	t.settings = map[string]settingValue{}
}

func (t *tracker) inTransaction(migration *ir.Migration) bool {
	return t.explicitTx || migration.TxMode == ir.TxModeTransactional
}

func (t *tracker) apply(stmt *ir.Statement) {
	switch stmt.Kind {
	case ir.StmtBegin:
		t.explicitTx = true
	case ir.StmtCommit, ir.StmtRollback:
		t.explicitTx = false
		t.clearLocalSettings()
	case ir.StmtSet:
		t.settings[stmt.Setting.Name] = settingValue{value: stmt.Setting.Value, local: stmt.Setting.Local}
	case ir.StmtReset:
		delete(t.settings, stmt.Setting.Name)
	default:
	}

	for _, effect := range stmt.Effects {
		t.applyEffect(effect)
	}
}

func (t *tracker) clearLocalSettings() {
	for name, setting := range t.settings {
		if setting.local {
			delete(t.settings, name)
		}
	}
}

func (t *tracker) applyEffect(effect ir.Effect) {
	switch effect.Kind {
	case ir.EffectCreateTable:
		t.newTables[tableKey(effect.Object)] = struct{}{}
	case ir.EffectDropTable:
		delete(t.newTables, tableKey(effect.Object))
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

func (t *tracker) renameTable(effect ir.Effect) {
	oldKey := tableKey(effect.Object)
	if _, ok := t.newTables[oldKey]; !ok {
		return
	}

	delete(t.newTables, oldKey)
	t.newTables[tableKey(ir.ObjectRef{Schema: effect.Object.Schema, Name: effect.NewName})] = struct{}{}
}
