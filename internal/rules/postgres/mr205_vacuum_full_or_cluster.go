package postgres

import (
	_ "embed"
	"fmt"
	"strings"

	pg_query "github.com/pganalyze/pg_query_go/v6"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
	"github.com/bbrainttech/migrail/internal/ir"
)

//go:embed mr205.md
var mr205Docs string

type vacuumFullOrCluster struct{}

func (vacuumFullOrCluster) Meta() ir.RuleMeta {
	return ir.RuleMeta{
		ID:              "MR205",
		Slug:            "vacuum-full-or-cluster",
		Title:           "VACUUM FULL and CLUSTER rewrite tables while blocking all queries",
		Category:        ir.CategoryRewrite,
		Dialects:        postgresOnly,
		DefaultSeverity: ir.SeverityError,
		DefaultEnabled:  true,
		Docs:            mr205Docs,
	}
}

func (vacuumFullOrCluster) Check(c *analyze.Context, stmt *ir.Statement) []ir.Finding {
	node, ok := pg.NodeOf(stmt)
	if !ok {
		return nil
	}

	switch {
	case node.Stmt.GetVacuumStmt() != nil && isVacuumFull(node.Stmt.GetVacuumStmt()):
		return rewriteFindings(c, "VACUUM FULL", lockmodel.VacuumFull, relationsOf(node.Stmt.GetVacuumStmt().GetRels()))
	case node.Stmt.GetClusterStmt() != nil:
		tables := []ir.ObjectRef{}
		if relation := node.Stmt.GetClusterStmt().GetRelation(); relation != nil {
			tables = append(tables, pg.RelationRef(relation))
		}

		return rewriteFindings(c, "CLUSTER", lockmodel.Cluster, tables)
	default:
		return nil
	}
}

func isVacuumFull(vacuum *pg_query.VacuumStmt) bool {
	if !vacuum.GetIsVacuumcmd() {
		return false
	}

	for _, option := range vacuum.GetOptions() {
		elem := option.GetDefElem()
		if !strings.EqualFold(elem.GetDefname(), "full") {
			continue
		}

		value := elem.GetArg()
		disabled := value.GetBoolean() != nil && !value.GetBoolean().GetBoolval() ||
			strings.EqualFold(value.GetString_().GetSval(), "false") || strings.EqualFold(value.GetString_().GetSval(), "off")

		return !disabled
	}

	return false
}

func rewriteFindings(c *analyze.Context, command string, op lockmodel.Operation, tables []ir.ObjectRef) []ir.Finding {
	fix := &ir.Fix{
		Summary:   "Rebuild the table online with pg_repack, or run this during a maintenance window:",
		Framework: langSQL,
		Steps:     []ir.FixStep{{Title: "Install the pg_repack extension and run it against the table instead"}},
	}

	if len(tables) == 0 {
		return []ir.Finding{{
			Title: command + " rewrites every table in the database",
			Why:   command + " without a table name processes every table, and holds an ACCESS EXCLUSIVE lock on each one while it rewrites it.",
			Fix:   fix,
		}}
	}

	existing := existingTables(c, tables)
	if len(existing) == 0 {
		return nil
	}

	names := make([]string, 0, len(existing))
	for _, table := range existing {
		names = append(names, quoted(table.String()))
	}

	return []ir.Finding{{
		Title: fmt.Sprintf("%s rewrites %s while blocking all queries", command, strings.Join(names, ", ")),
		Why:   command + " copies the whole table into new files and holds an ACCESS EXCLUSIVE lock until it finishes. Reads and writes wait the whole time.",
		Lock:  lockmodel.Impact(op, existing...),
		Fix:   fix,
	}}
}
