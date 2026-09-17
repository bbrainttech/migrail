# migrail

Catch dangerous database migrations before they take production down.

migrail reads your migrations the way your database will run them. It tells you which statements will lock a table, which will rewrite one, and which will break the code still running during a deploy. Each finding includes a fix written in your framework's syntax.

It's free and open source. You don't need an account or a config file, and it works on the migrations you already have.

> migrail is in early development and has no release yet. You can build it from source and check PostgreSQL SQL files today. The rest of this page describes v0.1, which is in progress.

## What it looks like

```
▌ db/migrations/20260917101500_add_orders_status.sql

  ✖ error MR101 Index creation blocks writes on "orders"

     ╭─ db/migrations/20260917101500_add_orders_status.sql:6:1
   6 │ CREATE INDEX idx_orders_status ON orders (status);
     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │
     ├─ lock    SHARE on orders · blocks INSERT, UPDATE, DELETE
     ├─ why     Postgres holds this lock until the whole index is built.
     ╰─ fix     Build the index concurrently, outside a transaction:

                -- +goose NO TRANSACTION
                -- +goose Up
                CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);
```

This is the target design for v0.1.

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
| golang-migrate, goose, Atlas, Flyway, Prisma, Drizzle, dbmate, sqitch | Planned for v0.1 |
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

Check one or more SQL migration files. Results are JSON for now; the terminal view shown above comes next.

```
./bin/migrail check db/migrations/20260917101500_add_orders_status.sql --db-version 16
```

Read SQL from standard input:

```
echo "ALTER TABLE users RENAME COLUMN email TO email_address;" | ./bin/migrail check -
```

| Flag | Meaning |
|---|---|
| `--db-version` | PostgreSQL major version you run in production, 12 to 18. Defaults to 12. |
| `--fail-on` | Exit with code 1 on findings at or above `error` (default), `warning` or `notice`. `never` always exits 0. |
| `-r`, `--rule` | Only run these rules, by ID or slug. |
| `--skip-rule` | Skip these rules, by ID or slug. |
| `-f`, `--format` | Output format. Only `json` is available today. |

Exit codes: `0` no failing findings, `1` findings at or above `--fail-on`, `2` usage error, `4` internal error.

### Available rules

| ID | Rule | Severity |
|---|---|---|
| MR101 | `create-index-non-concurrent`: `CREATE INDEX` without `CONCURRENTLY` blocks writes | error |
| MR104 | `add-foreign-key-validating`: a foreign key without `NOT VALID` blocks writes on both tables | error |
| MR107 | `set-not-null-scan`: `SET NOT NULL` scans the table while blocking all queries | error |
| MR201 | `alter-column-type-rewrite`: changing a column type rewrites the table | error |
| MR302 | `rename-column`: renaming a column breaks code that still uses the old name | error |
| MR901 | `parse-error`: the SQL can't be parsed | error |

Statements on tables created earlier in the same set of files don't trigger lock rules, because those tables are still empty.

## Roadmap

- [ ] Foundations: project setup, CI, cross-platform builds
- [x] Analysis engine: Postgres parser, first rules, JSON output
- [ ] Terminal output: colors, code frames, `explain` and `rules`
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
