# migrail

Catch dangerous database migrations before they take production down.

migrail reads your migrations the way your database will run them. It tells you which statements will lock a table, which will rewrite one, and which will break the code still running during a deploy. Each finding includes a fix written in your framework's syntax.

It's free and open source. You don't need an account or a config file, and it works on the migrations you already have.

> migrail is in early development and has no release yet. You can build it from source and check PostgreSQL SQL files today. Migration tool detection, git-aware checks and more rules arrive with v0.1.

## What it looks like

```
$ migrail check db/migrations/20260917101500_add_orders_index.sql --db-version 16

migrail 0.1.0 · postgres 16 · 1 file

▌ db/migrations/20260917101500_add_orders_index.sql

  ✖ error MR101 Index creation blocks writes on "orders"

     ╭─ db/migrations/20260917101500_add_orders_index.sql:2:1
   1 │ BEGIN;
   2 │ CREATE INDEX idx_orders_created_at ON orders (created_at);
     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │
     ├─ lock    SHARE on orders · blocks INSERT, UPDATE, DELETE
     ├─ why     Postgres holds this lock until the whole index is built. INSERT,
     │          UPDATE and DELETE on the table wait for the entire build.
     ╰─ fix     Build the index concurrently, outside a transaction:

                CREATE INDEX CONCURRENTLY idx_orders_created_at ON orders (created_at);

────────────────────────────────────────────────────────────────────────────────
  ✖ 1 error  ▲ 0 warnings  ● 0 notices                    1 file · 3 stmts · 6ms
  Failing: 1 finding at or above "error"
```

In a terminal, severities, table names and SQL are colored. Output adapts to the terminal width, and falls back to plain text with `NO_COLOR`, to ASCII symbols with `--ascii`, and to one line per finding below 60 columns.

## What it checks

- **Locks.** Statements that block reads or writes, such as an index built without `CONCURRENTLY` or a foreign key added without `NOT VALID`.
- **Table rewrites.** Column type changes and volatile defaults that rewrite the whole table under an exclusive lock.
- **Running code.** Dropping or renaming a column that your application still uses.
- **Transactions.** `CONCURRENTLY` inside a transaction, a missing `lock_timeout`, or validating a constraint in the same transaction that added it.
- **Data safety.** `TRUNCATE`, `DROP TABLE`, and `UPDATE` or `DELETE` without `WHERE`.

With an optional read-only connection to staging, migrail also estimates the impact from real table sizes, for example how long writes stay blocked.

## Why migrail

migrail is built around four commitments:

- **Your framework, not raw SQL.** Fixes use your framework's syntax. Rails users get `disable_ddl_transaction!` and `algorithm: :concurrently`. Django users get `AddIndexConcurrently` and `atomic = False`.
- **It knows your code.** Before you drop or rename a column, migrail will search your application for queries and models that still use it.
- **Lock claims are tested.** A test harness will run every lock and rewrite claim against real PostgreSQL 12 to 18 before it ships.
- **Every check is free.** The CLI and CI integration will have no paid tier and no login.

## Supported stacks

| Database | Status |
|---|---|
| PostgreSQL 12–18 | Planned for v0.1 |
| MySQL, MariaDB | Planned for v2 |
| SQL Server, SQLite, CockroachDB | Planned for v3 |

| Migration tool | Status |
|---|---|
| golang-migrate, goose, Atlas, Flyway, Prisma, Drizzle, dbmate, sqitch, plain `.sql` directories | Available from source |
| Django, Alembic, Rails, Laravel, EF Core, Knex, TypeORM, Sequelize, Liquibase | Planned for v0.2 |
| Any other tool, through capture mode | Planned for v0.2 |

## Installation

migrail has no release yet. It will ship as a single binary through Homebrew, npm, PyPI, RubyGems, Composer, NuGet, Scoop, Docker and a GitHub Action.

To build it from source you need Go 1.26.4 or newer and a C compiler:

```
git clone https://github.com/bbrainttech/migrail
cd migrail
make build
```

## Usage

From anywhere in your repository, find the migrations, detect the migration tool and check them:

```
./bin/migrail check --db-version 16
```

Check specific files or directories:

```
./bin/migrail check db/migrations/20260917101500_add_orders_status.sql --db-version 16
./bin/migrail check services/billing/migrations
```

migrail reads each tool's own conventions: goose `-- +goose Up` sections and `NO TRANSACTION`, dbmate `-- migrate:up` and `transaction:false`, Atlas `atlas:txmode`, the Prisma, Drizzle and sqitch plan order, Flyway versions and golang-migrate `.up.sql` files.

Read SQL from standard input:

```
echo "ALTER TABLE users RENAME COLUMN email TO email_address;" | ./bin/migrail check -
```

Learn what a rule detects and how to fix it, or list every rule:

```
./bin/migrail explain MR101
./bin/migrail rules
```

| `check` flag | Meaning |
|---|---|
| `--db-version` | PostgreSQL major version you run in production, 12 to 18. Defaults to 12. |
| `--fail-on` | Exit with code 1 on findings at or above `error` (default), `warning` or `notice`. `never` always exits 0. |
| `-r`, `--rule` | Only run these rules, by ID or slug. |
| `--skip-rule` | Skip these rules, by ID or slug. |
| `-f`, `--format` | `pretty` (default) or `json`. |
| `-d`, `--dir` | Project root to search for migrations. Defaults to the repository root. |
| `--framework` | Use this migration tool instead of detecting it: `goose`, `golang-migrate`, `atlas`, `flyway`, `prisma`, `drizzle`, `dbmate`, `sqitch` or `sql`. |
| `--compact` | One line per finding. |
| `-q`, `--quiet` | Only findings and the summary. |
| `-v`, `--verbose` | Print each phase with its timing. |
| `--ci` | No color, links or spinners unless `FORCE_COLOR` or `CLICOLOR_FORCE` is set. `CI=true` turns this on. |

| Global flag | Meaning |
|---|---|
| `--color` | `auto` (default), `always` or `never`. `NO_COLOR` is respected. |
| `--theme` | `auto` (default), `dark`, `light` or `mono`. |
| `--ascii` | ASCII symbols instead of Unicode. |
| `--no-hyperlinks` | Don't print clickable file links. |

Findings go to standard output. Notices and progress go to standard error, so `migrail check -f json > report.json` stays clean.

Exit codes: `0` no failing findings, `1` findings at or above `--fail-on`, `2` usage error, `4` internal error.

Shell completion: `migrail completion bash|zsh|fish|powershell`.

### Available rules

| ID | Rule | Severity |
|---|---|---|
| MR101 | `create-index-non-concurrent`: `CREATE INDEX` without `CONCURRENTLY` blocks writes | error |
| MR102 | `drop-index-non-concurrent`: `DROP INDEX` without `CONCURRENTLY` blocks all queries | warning |
| MR104 | `add-foreign-key-validating`: a foreign key without `NOT VALID` blocks writes on both tables | error |
| MR105 | `add-check-validating`: a `CHECK` constraint without `NOT VALID` blocks all queries | error |
| MR106 | `add-unique-constraint-blocking`: `UNIQUE` or `PRIMARY KEY` builds an index under an exclusive lock | error |
| MR107 | `set-not-null-scan`: `SET NOT NULL` scans the table while blocking all queries | error |
| MR201 | `alter-column-type-rewrite`: changing a column type rewrites the table | error |
| MR202 | `add-column-volatile-default`: a volatile default such as `gen_random_uuid()` rewrites the table | error |
| MR205 | `vacuum-full-or-cluster`: `VACUUM FULL` and `CLUSTER` rewrite tables while blocking all queries | error |
| MR207 | `add-column-not-null-no-default`: a `NOT NULL` column without a default fails on tables with rows | error |
| MR301 | `drop-column-in-use`: dropping a column breaks code that still uses it | warning |
| MR302 | `rename-column`: renaming a column breaks code that still uses the old name | error |
| MR303 | `rename-table`: renaming a table breaks code that still uses the old name | error |
| MR304 | `drop-table-in-use`: dropping a table breaks code that still uses it (off until code scanning) | warning |
| MR402 | `concurrent-in-transaction`: `CONCURRENTLY` fails inside a transaction | error |
| MR403 | `missing-lock-timeout`: locking DDL without `lock_timeout` can queue every query behind it | warning |
| MR404 | `not-valid-validate-same-tx`: validating in the same transaction as `NOT VALID` keeps the lock | error |
| MR501 | `truncate-table`: `TRUNCATE` deletes every row | error |
| MR502 | `drop-table`: `DROP TABLE` deletes the table and its data | warning |
| MR503 | `delete-or-update-without-where`: `UPDATE` or `DELETE` without `WHERE` changes every row | error |
| MR901 | `parse-error`: the SQL can't be parsed | error |
| MR903 | `dynamic-sql`: SQL inside a `DO` block can't be checked | notice |

Run `migrail explain <ID>` for what each rule detects, why it matters and how to fix it.

Statements on tables created earlier in the same set of files don't trigger lock rules, because those tables are still empty.

## Roadmap

- [ ] Foundations: project setup, CI, cross-platform builds
- [x] Analysis engine: Postgres parser, first rules, JSON output
- [x] Terminal output: colors, code frames, `explain` and `rules`
- [ ] v0.1: PostgreSQL rules and SQL migration tools
- [ ] v0.2: framework support, capture mode, CI integrations, package managers
- [ ] v0.3: application code checks, live database estimates, interactive explorer
- [ ] v1.0: full rule catalog, stable schemas, documentation site

## Contributing

Requirements: Go 1.26.4 or newer, a C compiler (the Postgres parser uses cgo), [golangci-lint](https://golangci-lint.run) v2, and Docker for integration tests.

```
make build      build bin/migrail
make test       run unit tests with the race detector
make lint       run golangci-lint
make fmt        format the code
make snapshot   build a release binary for this machine with goreleaser
```

## License

[Apache-2.0](LICENSE)
