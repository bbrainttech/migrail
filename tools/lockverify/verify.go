package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/bbrainttech/migrail/internal/dialect/postgres/lockmodel"
)

const (
	lockNotAvailable = "55P03"
	pollInterval     = 20 * time.Millisecond
	pollTimeout      = 10 * time.Second
)

var modes = []lockmodel.Mode{
	lockmodel.AccessShare, lockmodel.RowShare, lockmodel.RowExclusive, lockmodel.ShareUpdateExclusive,
	lockmodel.Share, lockmodel.ShareRowExclusive, lockmodel.Exclusive, lockmodel.AccessExclusive,
}

var pgModeNames = map[string]lockmodel.Mode{
	"AccessShareLock":          lockmodel.AccessShare,
	"RowShareLock":             lockmodel.RowShare,
	"RowExclusiveLock":         lockmodel.RowExclusive,
	"ShareUpdateExclusiveLock": lockmodel.ShareUpdateExclusive,
	"ShareLock":                lockmodel.Share,
	"ShareRowExclusiveLock":    lockmodel.ShareRowExclusive,
	"ExclusiveLock":            lockmodel.Exclusive,
	"AccessExclusiveLock":      lockmodel.AccessExclusive,
}

var probes = []struct {
	statement string
	sql       string
}{
	{statement: "SELECT", sql: "SELECT 1 FROM orders LIMIT 1"},
	{statement: "INSERT", sql: "INSERT INTO orders (id) VALUES (1000000)"},
	{statement: "UPDATE", sql: "UPDATE orders SET note = 'probe' WHERE id = 1"},
	{statement: "DELETE", sql: "DELETE FROM orders WHERE id = 2"},
}

const schema = `
DROP TABLE IF EXISTS orders, users CASCADE;
CREATE TABLE users (id bigint PRIMARY KEY);
CREATE TABLE orders (
  id bigint PRIMARY KEY,
  user_id bigint,
  status text NOT NULL DEFAULT 'new',
  total numeric NOT NULL DEFAULT 0,
  code text,
  label text,
  note text
);
INSERT INTO users SELECT n FROM generate_series(1, 200) AS s(n);
INSERT INTO orders (id, user_id, total, code, label)
  SELECT n AS id, n AS user_id, n AS total, 'c' || n AS code, 'x' AS label FROM generate_series(1, 200) AS s(n);
CREATE INDEX orders_status_idx ON orders (status);
`

type matrixResult struct {
	Mode     lockmodel.Mode `json:"mode"`
	Expected []string       `json:"expected"`
	Observed []string       `json:"observed"`
	OK       bool           `json:"ok"`
}

type operationResult struct {
	Operation       lockmodel.Operation `json:"operation"`
	Statement       string              `json:"statement"`
	ExpectedMode    lockmodel.Mode      `json:"expectedMode"`
	ObservedModes   string              `json:"observedModes"`
	ExpectedRewrite bool                `json:"expectedRewrite"`
	ObservedRewrite bool                `json:"observedRewrite"`
	Note            string              `json:"note,omitempty"`
	OK              bool                `json:"ok"`
}

type operationCase struct {
	operation     lockmodel.Operation
	prepare       []string
	statement     string
	tables        []string
	outsideTx     bool
	ignoreRewrite string
}

var cases = []operationCase{
	{operation: lockmodel.CreateIndex, statement: "CREATE INDEX orders_code_idx ON orders (code)", tables: onlyOrders},
	{operation: lockmodel.CreateIndexConcurrently, statement: "CREATE INDEX CONCURRENTLY orders_code_idx ON orders (code)", tables: onlyOrders, outsideTx: true},
	{operation: lockmodel.AddForeignKey, statement: "ALTER TABLE orders ADD CONSTRAINT orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id)", tables: []string{ordersTable, "users"}},
	{
		operation: lockmodel.ValidateConstraint,
		prepare:   []string{"ALTER TABLE orders ADD CONSTRAINT orders_user_id_fkey FOREIGN KEY (user_id) REFERENCES users (id) NOT VALID"},
		statement: "ALTER TABLE orders VALIDATE CONSTRAINT orders_user_id_fkey",
		tables:    []string{"orders"},
	},
	{operation: lockmodel.SetNotNull, statement: "ALTER TABLE orders ALTER COLUMN label SET NOT NULL", tables: onlyOrders},
	{operation: lockmodel.AlterColumnType, statement: "ALTER TABLE orders ALTER COLUMN total TYPE text", tables: onlyOrders},
	{operation: lockmodel.RenameColumn, statement: "ALTER TABLE orders RENAME COLUMN code TO order_code", tables: onlyOrders},
	{operation: lockmodel.DropIndex, statement: "DROP INDEX orders_status_idx", tables: onlyOrders},
	{operation: lockmodel.AddCheck, statement: "ALTER TABLE orders ADD CONSTRAINT orders_total_check CHECK (total >= 0)", tables: onlyOrders},
	{operation: lockmodel.AddUnique, statement: "ALTER TABLE orders ADD CONSTRAINT orders_code_key UNIQUE (code)", tables: onlyOrders},
	{operation: lockmodel.AddColumn, statement: "ALTER TABLE orders ADD COLUMN extra text DEFAULT 'x'", tables: onlyOrders},
	{operation: lockmodel.AddColumnRewrite, statement: "ALTER TABLE orders ADD COLUMN sample double precision DEFAULT random()", tables: onlyOrders},
	{operation: lockmodel.DropColumn, statement: "ALTER TABLE orders DROP COLUMN note", tables: onlyOrders},
	{operation: lockmodel.RenameTable, statement: "ALTER TABLE orders RENAME TO purchases", tables: onlyOrders},
	{operation: lockmodel.DropTable, statement: "DROP TABLE orders", tables: onlyOrders, ignoreRewrite: "the table no longer exists"},
	{operation: lockmodel.Truncate, statement: "TRUNCATE orders", tables: onlyOrders, ignoreRewrite: "TRUNCATE swaps in empty storage instead of rewriting rows"},
	{operation: lockmodel.VacuumFull, statement: "VACUUM FULL orders", tables: onlyOrders, outsideTx: true},
	{operation: lockmodel.Cluster, statement: "CLUSTER orders USING orders_pkey", tables: onlyOrders},
}

const ordersTable = "orders"

var onlyOrders = []string{ordersTable}

func execEach(ctx context.Context, conn *pgx.Conn, script string) error {
	for statement := range strings.SplitSeq(script, ";") {
		if strings.TrimSpace(statement) == "" {
			continue
		}

		if _, err := conn.Exec(ctx, statement); err != nil {
			return fmt.Errorf("%q: %w", strings.TrimSpace(statement), err)
		}
	}

	return nil
}

func closeQuietly(ctx context.Context, conn *pgx.Conn) {
	_ = conn.Close(ctx)
}

func connect(ctx context.Context, url string) (*pgx.Conn, error) {
	config, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse connection url: %w", err)
	}

	config.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}

	return conn, nil
}

func verifyMatrix(ctx context.Context, url string) ([]matrixResult, error) {
	holder, err := connect(ctx, url)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(ctx, holder)

	probe, err := connect(ctx, url)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(ctx, probe)

	if err := execEach(ctx, holder, schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}

	results := make([]matrixResult, 0, len(modes))

	for _, mode := range modes {
		if _, err := holder.Exec(ctx, fmt.Sprintf("BEGIN; LOCK TABLE orders IN %s MODE", mode)); err != nil {
			return nil, fmt.Errorf("lock in %s mode: %w", mode, err)
		}

		observed := []string{}

		for _, p := range probes {
			blocked, err := isBlocked(ctx, probe, p.sql)
			if err != nil {
				return nil, fmt.Errorf("probe %s under %s: %w", p.statement, mode, err)
			}

			if blocked {
				observed = append(observed, p.statement)
			}
		}

		if _, err := holder.Exec(ctx, "ROLLBACK"); err != nil {
			return nil, fmt.Errorf("release %s: %w", mode, err)
		}

		expected := mode.BlockedStatements()
		results = append(results, matrixResult{Mode: mode, Expected: expected, Observed: observed, OK: slices.Equal(expected, observed)})
	}

	return results, nil
}

func isBlocked(ctx context.Context, conn *pgx.Conn, sql string) (bool, error) {
	if _, err := conn.Exec(ctx, "BEGIN; SET LOCAL lock_timeout = '150ms'"); err != nil {
		return false, err
	}

	_, execErr := conn.Exec(ctx, sql)

	if _, err := conn.Exec(ctx, "ROLLBACK"); err != nil {
		return false, err
	}

	var pgErr *pgconn.PgError
	if errors.As(execErr, &pgErr) && pgErr.Code == lockNotAvailable {
		return true, nil
	}

	return false, execErr
}

func verifyOperations(ctx context.Context, url string) ([]operationResult, error) {
	results := make([]operationResult, 0, len(cases))

	for _, c := range cases {
		result, err := verifyOperation(ctx, url, c)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", c.operation, err)
		}

		results = append(results, result)
	}

	return results, nil
}

type relation struct {
	oid         uint32
	relfilenode uint32
}

func verifyOperation(ctx context.Context, url string, c operationCase) (operationResult, error) {
	observer, err := connect(ctx, url)
	if err != nil {
		return operationResult{}, err
	}
	defer closeQuietly(ctx, observer)

	if _, err := observer.Exec(ctx, schema+strings.Join(c.prepare, ";")); err != nil {
		return operationResult{}, fmt.Errorf("prepare: %w", err)
	}

	relations := map[string]relation{}

	for _, table := range c.tables {
		var rel relation
		if err := observer.QueryRow(ctx, "SELECT oid, relfilenode FROM pg_class WHERE relname = $1", table).Scan(&rel.oid, &rel.relfilenode); err != nil {
			return operationResult{}, fmt.Errorf("find %s: %w", table, err)
		}

		relations[table] = rel
	}

	worker, err := connect(ctx, url)
	if err != nil {
		return operationResult{}, err
	}
	defer closeQuietly(ctx, worker)

	observed, rewritten, err := runOperation(ctx, url, observer, worker, c, relations)
	if err != nil {
		return operationResult{}, err
	}

	entry, _ := lockmodel.Lookup(c.operation)
	result := operationResult{
		Operation:       c.operation,
		Statement:       c.statement,
		ExpectedMode:    entry.Mode,
		ExpectedRewrite: entry.Rewrite,
		ObservedRewrite: rewritten,
		Note:            c.ignoreRewrite,
	}

	names := []string{}
	modesOK := true

	for _, table := range c.tables {
		names = append(names, fmt.Sprintf("%s=%s", table, observed[table]))
		modesOK = modesOK && observed[table] == entry.Mode
	}

	sort.Strings(names)
	result.ObservedModes = strings.Join(names, ",")
	result.OK = modesOK && (c.ignoreRewrite != "" || rewritten == entry.Rewrite)

	return result, nil
}

func runOperation(
	ctx context.Context,
	url string,
	observer, worker *pgx.Conn,
	c operationCase,
	relations map[string]relation,
) (map[string]lockmodel.Mode, bool, error) {
	primary := relations[c.tables[0]]
	pid := worker.PgConn().PID()

	if !c.outsideTx {
		return runInTransaction(ctx, observer, worker, c, relations, primary, pid)
	}

	return runOutsideTransaction(ctx, url, observer, worker, c, relations)
}

func runInTransaction(
	ctx context.Context,
	observer, worker *pgx.Conn,
	c operationCase,
	relations map[string]relation,
	primary relation,
	pid uint32,
) (map[string]lockmodel.Mode, bool, error) {
	if _, err := worker.Exec(ctx, "BEGIN"); err != nil {
		return nil, false, err
	}

	if _, err := worker.Exec(ctx, c.statement); err != nil {
		return nil, false, fmt.Errorf("run %q: %w", c.statement, err)
	}

	observed, err := heldModes(ctx, observer, pid, relations)
	if err != nil {
		return nil, false, err
	}

	var after uint32

	rewritten := false
	if err := worker.QueryRow(ctx, "SELECT relfilenode FROM pg_class WHERE oid = $1", primary.oid).Scan(&after); err == nil {
		rewritten = after != primary.relfilenode
	}

	if _, err := worker.Exec(ctx, "ROLLBACK"); err != nil {
		return nil, false, err
	}

	return observed, rewritten, nil
}

func runOutsideTransaction(
	ctx context.Context,
	url string,
	observer, worker *pgx.Conn,
	c operationCase,
	relations map[string]relation,
) (map[string]lockmodel.Mode, bool, error) {
	blocker, err := connect(ctx, url)
	if err != nil {
		return nil, false, err
	}
	defer closeQuietly(ctx, blocker)

	if _, err := blocker.Exec(ctx, "BEGIN ISOLATION LEVEL REPEATABLE READ; SELECT count(*) FROM orders"); err != nil {
		return nil, false, fmt.Errorf("start blocker: %w", err)
	}

	pid := worker.PgConn().PID()
	done := make(chan error, 1)

	go func() {
		_, err := worker.Exec(ctx, c.statement)
		done <- err
	}()

	observed := map[string]lockmodel.Mode{}
	deadline := time.Now().Add(pollTimeout)

	for time.Now().Before(deadline) && len(observed) < len(c.tables) {
		observed, err = heldModes(ctx, observer, pid, relations)
		if err != nil {
			return nil, false, err
		}

		time.Sleep(pollInterval)
	}

	if _, err := blocker.Exec(ctx, "COMMIT"); err != nil {
		return nil, false, fmt.Errorf("release blocker: %w", err)
	}

	if err := <-done; err != nil {
		return nil, false, fmt.Errorf("run %q: %w", c.statement, err)
	}

	var after uint32
	if err := observer.QueryRow(ctx, "SELECT relfilenode FROM pg_class WHERE oid = $1", relations[c.tables[0]].oid).Scan(&after); err != nil {
		return nil, false, fmt.Errorf("read relfilenode: %w", err)
	}

	return observed, after != relations[c.tables[0]].relfilenode, nil
}

func heldModes(ctx context.Context, observer *pgx.Conn, pid uint32, relations map[string]relation) (map[string]lockmodel.Mode, error) {
	observed := map[string]lockmodel.Mode{}

	for table, rel := range relations {
		rows, err := observer.Query(ctx, "SELECT mode FROM pg_locks WHERE pid = $1 AND relation = $2", pid, rel.oid)
		if err != nil {
			return nil, fmt.Errorf("read pg_locks: %w", err)
		}

		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return nil, fmt.Errorf("read pg_locks: %w", err)
		}

		for _, name := range names {
			mode := pgModeNames[name]
			if current, ok := observed[table]; !ok || slices.Index(modes, mode) > slices.Index(modes, current) {
				observed[table] = mode
			}
		}
	}

	return observed, nil
}
