package livedb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/bbrainttech/migrail/internal/ir"
)

const (
	applicationName = "migrail"
	connectTimeout  = 5 * time.Second
	closeTimeout    = 2 * time.Second

	versionBase = 10000
)

var ErrInvalidURL = errors.New("the database URL isn't valid")

const tablesQuery = `
SELECT refs.ref,
       COALESCE(sum(CASE WHEN child.reltuples > 0 THEN child.reltuples ELSE COALESCE(stat.n_live_tup, 0) END), 0)::bigint,
       COALESCE(sum(pg_table_size(child.oid)), 0)::bigint,
       COALESCE(sum(pg_indexes_size(child.oid)), 0)::bigint
FROM json_array_elements_text($1::json) AS refs(ref)
JOIN pg_class AS parent ON parent.oid = to_regclass(refs.ref)
CROSS JOIN LATERAL (
  SELECT parent.oid AS relid
  UNION
  SELECT tree.relid FROM pg_partition_tree(parent.oid) AS tree
) AS members
JOIN pg_class AS child ON child.oid = members.relid
LEFT JOIN pg_stat_all_tables AS stat ON stat.relid = child.oid
GROUP BY refs.ref`

const longTransactionsQuery = `
SELECT count(*), COALESCE(EXTRACT(EPOCH FROM max(now() - xact_start)), 0)::bigint
FROM pg_stat_activity
WHERE xact_start < now() - make_interval(secs => $1::double precision)
  AND pid <> pg_backend_pid()`

type Session struct {
	conn    *pgx.Conn
	tx      pgx.Tx
	url     string
	Version ir.Version
}

type Table struct {
	Rows       int64
	TableBytes int64
	IndexBytes int64
}

func (t Table) TotalBytes() int64 {
	return t.TableBytes + t.IndexBytes
}

type LongTransactions struct {
	Count  int
	Oldest time.Duration
}

func Open(ctx context.Context, databaseURL string) (*Session, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, ErrInvalidURL
	}

	cfg.ConnectTimeout = connectTimeout
	cfg.RuntimeParams["application_name"] = applicationName
	cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, redactedError("connect to the database", err, databaseURL)
	}

	session := &Session{conn: conn, url: databaseURL}

	if err := session.begin(ctx); err != nil {
		session.Close(ctx)

		return nil, err
	}

	return session, nil
}

func (s *Session) begin(ctx context.Context) error {
	tx, err := s.conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return redactedError("start a read-only transaction", err, s.url)
	}

	s.tx = tx

	for _, statement := range []string{"SET LOCAL statement_timeout = '5s'", "SET LOCAL lock_timeout = '1s'"} {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return redactedError("set session timeouts", err, s.url)
		}
	}

	var versionNumber string
	if err := tx.QueryRow(ctx, "SHOW server_version_num").Scan(&versionNumber); err != nil {
		return redactedError("read the server version", err, s.url)
	}

	version, err := parseVersionNumber(versionNumber)
	if err != nil {
		return err
	}

	s.Version = version

	return nil
}

func parseVersionNumber(text string) (ir.Version, error) {
	number, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return ir.Version{}, fmt.Errorf("read the server version: %q isn't a version number", text)
	}

	return ir.Version{Major: number / versionBase, Minor: number % versionBase}, nil
}

func (s *Session) Close(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), closeTimeout)
	defer cancel()

	if s.tx != nil {
		_ = s.tx.Rollback(ctx)
	}

	_ = s.conn.Close(ctx)
}

func (s *Session) Tables(ctx context.Context, names []string) (map[string]Table, error) {
	tables := map[string]Table{}
	if len(names) == 0 {
		return tables, nil
	}

	byReference := make(map[string]string, len(names))
	references := make([]string, 0, len(names))

	for _, name := range names {
		reference := qualifiedReference(name)
		byReference[reference] = name
		references = append(references, reference)
	}

	encoded, err := json.Marshal(references)
	if err != nil {
		return nil, fmt.Errorf("encode table names: %w", err)
	}

	rows, err := s.tx.Query(ctx, tablesQuery, string(encoded))
	if err != nil {
		return nil, redactedError("read table sizes", err, s.url)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			reference string
			table     Table
		)

		if err := rows.Scan(&reference, &table.Rows, &table.TableBytes, &table.IndexBytes); err != nil {
			return nil, redactedError("read table sizes", err, s.url)
		}

		tables[byReference[reference]] = table
	}

	if err := rows.Err(); err != nil {
		return nil, redactedError("read table sizes", err, s.url)
	}

	return tables, nil
}

func (s *Session) LongTransactions(ctx context.Context, olderThan time.Duration) (LongTransactions, error) {
	var (
		result  LongTransactions
		seconds int64
	)

	row := s.tx.QueryRow(ctx, longTransactionsQuery, olderThan.Seconds())
	if err := row.Scan(&result.Count, &seconds); err != nil {
		return LongTransactions{}, redactedError("read open transactions", err, s.url)
	}

	result.Oldest = time.Duration(seconds) * time.Second

	return result, nil
}

func qualifiedReference(name string) string {
	schema, table, qualified := strings.Cut(name, ".")
	if !qualified {
		return pgx.Identifier{name}.Sanitize()
	}

	return pgx.Identifier{schema, table}.Sanitize()
}

var (
	urlCredentials = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^@\s/]*@`)
	passwordField  = regexp.MustCompile(`(?i)(password|sslpassword)\s*=\s*\S+`)
)

func Redact(text, databaseURL string) string {
	redacted := text

	if databaseURL != "" {
		redacted = strings.ReplaceAll(redacted, databaseURL, "[database URL]")

		if parsed, err := url.Parse(databaseURL); err == nil && parsed.User != nil {
			if password, ok := parsed.User.Password(); ok && password != "" {
				redacted = strings.ReplaceAll(redacted, password, "[redacted]")
			}
		}
	}

	redacted = urlCredentials.ReplaceAllString(redacted, "$1[redacted]@")

	return passwordField.ReplaceAllString(redacted, "$1=[redacted]")
}

func redactedError(action string, err error, databaseURL string) error {
	if errors.Is(err, context.Canceled) {
		return err
	}

	return fmt.Errorf("%s: %s", action, Redact(err.Error(), databaseURL))
}
