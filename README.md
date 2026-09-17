# migrail

Catch dangerous database migrations before they take production down.

migrail reads your migrations the way your database will run them. It tells you what will lock, what will rewrite a table, and what will break the code that's still running during a deploy. Every finding comes with a fix written for your framework.

Free and open source. No account, no config file, and it works with the migrations you already have.

> **Status: in development.** Nothing is released yet. This README grows as features land. [PLAN.md](PLAN.md) describes everything that's planned.

## Example

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

This shows the planned output. It isn't implemented yet.

## What it will check

- **Locks:** statements that block reads or writes, such as an index built without `CONCURRENTLY` or a foreign key added without `NOT VALID`
- **Table rewrites:** column type changes and volatile defaults that rewrite the whole table under an exclusive lock
- **Running code:** dropping or renaming a column that your application still uses
- **Transactions:** `CONCURRENTLY` inside a transaction, missing `lock_timeout`, validating a constraint in the same transaction that added it
- **Data safety:** `TRUNCATE`, `DROP TABLE`, and `UPDATE` or `DELETE` without `WHERE`

With an optional read-only connection to staging, it also estimates the real impact from table sizes.

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
| Anything else | Planned for v0.2 through capture mode |

## Installation

Not available yet. migrail will ship as a single binary through Homebrew, npm, PyPI, RubyGems, Composer, NuGet, Scoop, Docker, and as a GitHub Action.

## Roadmap

- [ ] **M0** Foundations
- [ ] **M1** Analysis engine
- [ ] **M2** Terminal output
- [ ] **M3** v0.1: PostgreSQL and SQL migration tools
- [ ] **M4** v0.2: framework support, capture mode, CI integrations, package managers
- [ ] **M5** v0.3: application code checks, live database mode, interactive explorer
- [ ] **M6** v1.0

## Development

Requirements:

- Go 1.24 or newer
- A C compiler (the Postgres parser uses cgo)
- Docker, for integration tests

Build and test commands will be documented here once the project is scaffolded.

## License

Apache-2.0 (planned).
