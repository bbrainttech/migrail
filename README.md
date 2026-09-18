# migrail

migrail checks your database migrations before you deploy them. It tells you which statements will lock a table, rewrite it or break the running app, and shows how to fix each one.

It's a free, open source CLI for PostgreSQL. No account, no config file, no database connection needed.

```
$ migrail check --db-version 16

migrail 0.1.0 · postgres 16 · 1 file

▌ db/migrations/20260917101500_add_orders_index.sql

  ✖ error MR101 Index creation blocks writes on "orders"

     ╭─ db/migrations/20260917101500_add_orders_index.sql:2:1
   1 │ SET lock_timeout = '5s';
   2 │ CREATE INDEX idx_orders_created_at ON orders (created_at);
     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │
     ├─ lock    SHARE on orders · blocks INSERT, UPDATE, DELETE
     ├─ why     Postgres holds this lock until the whole index is built. INSERT,
     │          UPDATE and DELETE on the table wait for the entire build.
     ╰─ fix     Build the index concurrently, outside a transaction:

                CREATE INDEX CONCURRENTLY idx_orders_created_at ON orders (created_at);

────────────────────────────────────────────────────────────────────────────────
  ✖ 1 error  ▲ 0 warnings  ● 0 notices                   1 file · 2 stmts · 6ms
  Failing: 1 finding at or above "error"
```

## Get started

**1. Install**

```
brew install bbrainttech/tap/migrail
```

Or, on macOS and Linux without Homebrew:

```
curl -fsSL https://github.com/bbrainttech/migrail/releases/latest/download/install.sh | sh
```

On Windows, download the zip from the [releases page](https://github.com/bbrainttech/migrail/releases) and put `migrail.exe` on your `PATH`.

**2. Run it in your repository**

```
migrail check --db-version 16
```

Replace `16` with the PostgreSQL version you run in production. migrail finds your migrations, detects your migration tool and checks the migrations you added or changed compared with your main branch. Add `--all` to check every migration.

**3. Fix what it reports**

Each finding shows the lock, why it's a problem and the fix. For more detail on a rule:

```
migrail explain MR101
```

## Use it in CI

For GitHub Actions, add this workflow. It fails the pull request on errors, marks each finding on the changed line, and keeps one comment on the pull request up to date:

```yaml
# .github/workflows/migrail.yml
name: migrail
on: pull_request

permissions:
  contents: read
  pull-requests: write

jobs:
  migrail:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: bbrainttech/migrail@v0.2.0
        with:
          db-version: "16"
```

`fetch-depth: 0` lets migrail compare against the base branch. The action's inputs:

| Input | Default | Meaning |
| --- | --- | --- |
| `db-version` | `12` | PostgreSQL version you run in production. |
| `fail-on` | `error` | Fail on findings at or above `error`, `warning` or `notice`. `never` only reports. |
| `args` | | Extra arguments for `migrail check`, such as a path or `--all`. |
| `comment` | `true` | Post the report as a pull request comment and update it on each push. |
| `sarif` | `false` | Upload findings to GitHub code scanning. Needs `security-events: write`. |
| `working-directory` | `.` | Directory to run in. |
| `version` | the action's tag | migrail release to install. |

On other CI systems, install migrail and run `migrail check`. Write reports for your CI with `-o`, for example a JUnit report for GitLab or Jenkins:

```
migrail check -o junit=reports/migrail.xml
```

> The GitHub Action, `-o` and the `sarif`, `github`, `markdown` and `junit` formats ship in v0.2.0, the next release. Until then, [build from source](#build-from-source) to try them.

## What it catches

| Problem | Example | Rule |
| --- | --- | --- |
| Blocks writes while an index builds | `CREATE INDEX` without `CONCURRENTLY` | MR101 |
| Blocks queries while an index is dropped | `DROP INDEX` without `CONCURRENTLY` | MR102 |
| Scans a table under a lock | foreign key or `CHECK` without `NOT VALID`, `SET NOT NULL`, `ADD UNIQUE` | MR104 to MR107 |
| Rewrites the whole table | column type change, volatile default, `VACUUM FULL` | MR201, MR202, MR205 |
| Fails on tables with rows | `NOT NULL` column without a default | MR207 |
| Breaks the running app | renaming or dropping a column or table | MR301 to MR304 |
| Fails or holds locks in a transaction | `CONCURRENTLY` in a transaction, no `lock_timeout`, validating in the same transaction | MR402 to MR404 |
| Deletes data | `TRUNCATE`, `DROP TABLE`, `UPDATE` or `DELETE` without `WHERE` | MR501 to MR503 |
| Edited a migration that already ran | changing a file that is on the base branch | MR401 |

Run `migrail rules` for the full list with severities.

Statements on tables created earlier in the same set of migrations aren't reported, because those tables are still empty.

## Supported tools

migrail supports PostgreSQL 12 to 18 and reads each tool's own conventions, including which migrations run in a transaction:

- golang-migrate
- goose
- Atlas
- Flyway
- Prisma
- Drizzle
- dbmate
- sqitch
- plain `.sql` folders

Django, Rails, Laravel, Alembic, EF Core, Knex, TypeORM, Sequelize and Liquibase are planned for v0.2. MySQL and MariaDB are planned for v2.

## Common tasks

Check specific files or folders:

```
migrail check db/migrations/20260917101500_add_status.sql
migrail check services/billing/migrations
```

Check SQL from another tool:

```
alembic upgrade head --sql | migrail check -
```

Ignore a finding that is safe in your case. The reason is required:

```sql
-- migrail:ignore MR101 reason="orders is empty until launch"
CREATE INDEX idx_orders_status ON orders (status);
```

`migrail:ignore` applies to the next statement, and `migrail:ignore-file` to the whole file. Without a reason, the finding is still reported.

Get machine-readable output:

```
migrail check -f json > report.json
```

## Options

| `check` flag | Meaning |
| --- | --- |
| `--db-version` | PostgreSQL version you run in production, 12 to 18. Defaults to 12. |
| `--all` | Check every migration, not only the ones changed since the base branch. |
| `--base` | Branch or commit to compare against. Defaults to `origin/HEAD`, then `main` or `master`. |
| `--fail-on` | Exit with code 1 on findings at or above `error` (default), `warning` or `notice`. `never` always exits 0. |
| `-f`, `--format` | Output format: `pretty` (default), `json`, `sarif`, `github`, `markdown` or `junit`. |
| `-o`, `--output` | Also write a report to a file. The format comes from the extension (`.sarif`, `.json`, `.md`, `.xml` for JUnit, `.txt`) or a prefix, such as `junit=report.xml`. Repeatable. |
| `-r`, `--rule` | Only run these rules, by ID or name. |
| `--skip-rule` | Skip these rules, by ID or name. |
| `-d`, `--dir` | Project root to search. Defaults to the repository root. |
| `--framework` | Use this migration tool instead of detecting it. |
| `--compact` | One line per finding. |
| `-q`, `--quiet` | Only findings and the summary. |

Global flags: `--color auto|always|never` (`NO_COLOR` works too), `--theme auto|dark|light|mono`, `--ascii`, `--no-hyperlinks`. Run `migrail check --help` for everything.

Exit codes: `0` passed, `1` findings at or above `--fail-on`, `2` usage or config error, `4` internal error.

In GitHub Actions, `migrail check` also adds annotations and a job summary on its own.

## Configuration

You don't need a config file. To share settings with your team, add `.migrail.yaml` at the repository root:

```yaml
version: 1

db:
  version: "16"

rules:
  MR403: off
  create-index-non-concurrent: warning

policy:
  fail_on: error

ignore:
  - path: "db/migrations/2019*"
    reason: applied long ago
```

Flags override the config file. `migrail schema config` prints the JSON schema for editor autocompletion. The full set of keys:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/bbrainttech/migrail/main/schemas/config.schema.json
version: 1
db:
  version: "16"
projects:                     # omit to detect projects automatically
  - path: services/api
    framework: goose
    migrations: db/migrations
git:
  base: origin/main
  check: changed              # changed or all
rules:
  MR403: off                  # off, notice, warning or error
policy:
  fail_on: error              # error, warning, notice or never
  require_ignore_reason: true
ignore:
  - rule: MR502
    path: "legacy/**"
    reason: legacy tables are dropped on purpose
output:
  format: pretty              # pretty, json, sarif, github, markdown or junit
  theme: auto
```

## Install and uninstall

| Method | Install | Uninstall |
| --- | --- | --- |
| Homebrew | `brew install bbrainttech/tap/migrail` | `brew uninstall migrail` |
| Install script | `curl -fsSL https://github.com/bbrainttech/migrail/releases/latest/download/install.sh \| sh` | `rm ~/.local/bin/migrail` |
| Windows | download the zip from the [releases page](https://github.com/bbrainttech/migrail/releases) | delete `migrail.exe` |

The install script puts migrail in `~/.local/bin`. Pass `--version v0.1.0` to pin a release or `--dir` to choose the folder. Releases after v0.1.0 also accept `--uninstall`. migrail writes no other files, so removing the binary uninstalls it.

Each release has `checksums.txt`, a cosign signature for it and an SBOM for every archive. The install script checks the checksum.

### Build from source

You need Go 1.26.4 or newer and a C compiler:

```
git clone https://github.com/bbrainttech/migrail
cd migrail
make build
./bin/migrail version
```

## Roadmap

- [x] v0.1: PostgreSQL rules and SQL migration tools
- [ ] v0.2: framework support (Django, Rails, Laravel and more), CI integrations, package managers
- [ ] v0.3: checks against your application code, estimates from a read-only staging database, interactive explorer
- [ ] v1.0: full rule catalog, stable schemas, documentation site

## Contributing

You need Go 1.26.4 or newer, a C compiler, [golangci-lint](https://golangci-lint.run) v2, and Docker for the lock tests.

```
make build           build bin/migrail
make test            run unit tests with the race detector
make lint            run golangci-lint
make fmt             format the code
make golden-update   regenerate output snapshots, then review the diff
make lockverify      check the lock model against PostgreSQL 12 to 18 (needs Docker)
make snapshot        build a release binary for this machine
```

## License

[Apache-2.0](LICENSE)
