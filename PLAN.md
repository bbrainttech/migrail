# migrail — Master Plan

> **Catch dangerous database migrations before they take production down.**
> Free, open source, zero-config, and built for every backend stack.

Name: `migrail`. Repository owner, docs domain and branding are decided later (see [§25 Open decisions](#25-open-decisions)). Placeholders: `<owner>` for the GitHub user or org, `<docs-domain>` for the docs site.

---

## Table of contents

1. [Vision](#1-vision)
2. [Competitive landscape & how we win](#2-competitive-landscape--how-we-win)
3. [Principles](#3-principles)
4. [Users & core use cases](#4-users--core-use-cases)
5. [Scope by release](#5-scope-by-release)
6. [System architecture](#6-system-architecture)
7. [Repository layout](#7-repository-layout)
8. [Core data model](#8-core-data-model)
9. [Analysis pipeline](#9-analysis-pipeline)
10. [Dialects & lock models](#10-dialects--lock-models)
11. [Rule catalog](#11-rule-catalog)
12. [Framework adapters](#12-framework-adapters)
13. [Capture mode (universal adapter)](#13-capture-mode-universal-adapter)
14. [Code usage scanner](#14-code-usage-scanner)
15. [Live database mode](#15-live-database-mode)
16. [Configuration](#16-configuration)
17. [Suppressions & baselines](#17-suppressions--baselines)
18. [CLI specification](#18-cli-specification)
19. [Terminal design system](#19-terminal-design-system)
20. [Output formats & CI integrations](#20-output-formats--ci-integrations)
21. [Performance](#21-performance)
22. [Distribution](#22-distribution)
23. [Testing & quality](#23-testing--quality)
24. [Security, privacy, docs, community](#24-security-privacy-docs-community)
25. [Open decisions](#25-open-decisions)
26. [Milestones & acceptance criteria](#26-milestones--acceptance-criteria)
27. [Future (post-1.0)](#27-future-post-10)

---

## 1. Vision

Every week, a team somewhere takes production down with a migration that looked harmless in code review:
an index built without `CONCURRENTLY`, a column rename while old pods are still serving traffic, or a type change that rewrites a 50 GB table under an exclusive lock.

`migrail` reads your migrations **the way your database will execute them** and tells you, before merge:

- **What will lock**, which operations it blocks, and for how long (estimated from real table sizes when available).
- **What will break the running app** during a rolling deploy (it cross-checks your application code).
- **Exactly how to do it safely**, written in *your* framework's syntax, not generic SQL.

It runs anywhere: a single fast binary with a terminal UI people enjoy using, installable through every ecosystem's package manager, plus first-class CI integration.

### Success looks like
- A developer on Django, Rails, Laravel, Prisma, Spring, .NET, Node or Go runs **one command** and gets a correct, useful result in **under a second** with **no config file**.
- False positive rate on real-world open source migration corpora is **< 5%** for `error` severity.
- Teams leave it enabled in CI because it's fast, quiet when things are fine, and precise when they aren't.

---

## 2. Competitive landscape & how we win

Audited 2026-09-17 against each project's own docs, release notes and GitHub repository. Star counts are from the GitHub API on that date.

| Tool | What it does well | Gaps we exploit |
|---|---|---|
| **Atlas** (atlasgo.io, v1.3.0, 8.7k stars, Apache-2.0) | Schema management for many databases, declarative and versioned migrations, reads golang-migrate/goose/Flyway/Liquibase/dbmate directories. About 60 lint checks, including Postgres lock and rewrite checks (PG101–PG311) and MySQL copy/rebuild checks | Since v0.38 (Oct 2025) `atlas migrate lint` is **Atlas Pro only** in the official binary and needs `atlas login`. The Community build keeps the older lint engine. Every Postgres lock/rewrite check (PG1xx, PG3xx) is marked Pro. Lint needs a dev database (`--dev-url`). No application code awareness. Fixes are generic SQL |
| **squawk** (v2.65.0, 1.2k stars, Apache-2.0/MIT) | Fast Postgres SQL linter (Rust, real Postgres parser), 42 rules, `--pg-version`, `--assume-in-transaction`, GitHub Action and PR comments, VS Code extension, npm/pip/Docker installs | Postgres and SQL files only. Its docs say it "doesn't have special support for Django or any other web ORM"; users pipe generated SQL in (`alembic upgrade --sql \| squawk`). No code awareness. No live table sizes |
| **strong_migrations** (4.4k stars, MIT) | Excellent Rails-native checks | Rails only, runtime-only (checks when migration runs) |
| **django-pg-zero-downtime-migrations** (0.6k stars, MIT) | Safe Django migration backend | Django + Postgres only; changes execution rather than reviewing |
| **pgroll** (6.6k stars, Apache-2.0) / Reshape | Zero-downtime execution via expand/contract | Different category (executor); requires adopting their migration format |
| **eugene** (62 stars, MIT) | Postgres linter plus a trace mode that runs migrations against a temporary database and reports the locks actually taken | Postgres and SQL only; no framework adapters; no code awareness |
| **MigrationPilot** (7 stars, MIT) | Postgres linter on the real parser, claims 112 rules, lock analysis, auto-fix | Postgres and SQL only; some rules sold as a paid tier; very early |

What this changes for us:
- The free official Atlas binary no longer lints, and its Postgres lock checks were already paid. A free tool with verified lock claims fills that gap. We still compare against the Community build honestly.
- squawk is the closest free tool. We don't win on "has a Postgres linter". We win on framework adapters, application code checks, live impact estimates and framework-native fixes. We need parity on its basics too: `--pg-version`, assumed transaction mode, GitHub PR comments, an editor extension.
- eugene's trace mode shows observed locks are valued. Our lock harness (§23.3) must stay a visible part of the docs.

### Our wedge: free, better, easier

1. **100% free and open source.** No account, no login, no cloud requirement, no feature gating in the CLI or CI. License: Apache-2.0.
2. **Zero adoption cost.** Works on the migrations you *already have*, in the framework you *already use*. No new migration format or workflow.
3. **Zero config.** `migrail` with no arguments auto-detects the framework, the migrations directory, the git base branch and the database dialect.
4. **App-aware.** It's the only tool that checks whether your *application code* still uses a column you're dropping or renaming (see §14).
5. **Framework-native fixes.** It suggests `disable_ddl_transaction!` + `algorithm: :concurrently` to Rails users and `AddIndexConcurrently` + `atomic = False` to Django users, not raw SQL.
6. **Real-world impact.** With an optional read-only DB connection: "`orders` has 41M rows (18 GB); this rewrite will hold `ACCESS EXCLUSIVE` for roughly 4–9 minutes."
7. **Verified lock claims.** Every lock/rewrite claim is empirically verified by our test harness against real database versions (see §23.3). Ours is backed by tests, not only by docs.
8. **Delightful terminal UX.** Compiler-grade diagnostics (like Rust/Elm error messages), an interactive explorer, and a setup wizard.

### Dialect strategy vs Atlas's breadth
Atlas is broad. We win by being **deeper per dialect first**, then expanding:
- **v1: PostgreSQL** (the most common choice for new backends, and the one with the most dangerous DDL footguns).
- **v2: MySQL + MariaDB.**
- **v3: SQL Server, SQLite, CockroachDB.**
- Later: Oracle, ClickHouse (by demand).

The architecture is dialect-pluggable from day one (§10), so expanding never requires a rewrite.

---

## 3. Principles

1. **Correct before clever.** A wrong "safe" is worse than no tool. When uncertain, say so ("could not determine transaction mode: assuming transactional").
2. **Quiet when safe.** A clean run prints one line. The migrail artwork appears only on welcome screens (`--help`, `init`, `version`; §19.5 J), never above `check` results, in CI or in piped output.
3. **Every finding is actionable.** Each one answers *what*, *where*, *why it's dangerous* and *how to fix it*, in the user's framework.
4. **Fast enough to never think about.** Sub-second on typical repos; results start streaming immediately.
5. **Read-only, always.** migrail never writes to a user's database. Capture mode only ever touches a throwaway database it created itself.
6. **Works offline.** No network needed except for optional features that explicitly require it (capture mode image pull, live DB).
7. **Beautiful and accessible.** Rich colors when supported, perfect plain text when not. Never rely on color alone to convey meaning.
8. **Single source of truth.** Rule metadata and docs live next to the rule code, and generate the CLI `explain` output, the docs site, SARIF metadata and the JSON schema.

---

## 4. Users & core use cases

### Personas
- **Backend developer:** writes a migration, wants to know before pushing if it's safe.
- **Reviewer / tech lead:** wants the PR to say clearly "this migration is dangerous because…".
- **Platform / SRE team:** wants org-wide policy (e.g. "no table rewrites on tables > 1M rows without approval").
- **Solo / indie dev:** wants to avoid mistakes without learning Postgres lock internals.

### Use cases
| # | Scenario | Command |
|---|---|---|
| U1 | Check my new migration locally | `migrail` |
| U2 | Check in CI and fail the build on errors | `migrail check --ci` |
| U3 | Understand why a rule fired | `migrail explain MR101` |
| U4 | Explore findings interactively and ignore false positives | `migrail check -i` |
| U5 | Set up in a new repo | `migrail init` |
| U6 | Check migrations from a framework without a native adapter | `migrail capture -- <migrate command>` |
| U7 | Estimate real impact against staging | `migrail check --db $STAGING_URL` |
| U8 | Adopt in a legacy repo without drowning in old findings | `migrail baseline` |
| U9 | Check a raw SQL snippet | `echo "ALTER TABLE…" \| migrail check -` |
| U10 | Diagnose setup problems | `migrail doctor` |

---

## 5. Scope by release

### v0.1 (MVP: "Postgres SQL, beautiful")
- Postgres dialect, pg_query parser.
- Raw SQL adapters: golang-migrate, goose, Atlas-format dirs, Flyway (SQL), Prisma, Drizzle, dbmate, sqitch (deploy scripts), plain `.sql` directories.
- ~20 core rules (§11 items marked **v0.1**).
- Git-aware change detection.
- Pretty terminal output, JSON output, exit codes.
- `check`, `explain`, `rules`, `version`, `completion`.
- Config file + inline suppressions.

### v0.2 ("Every framework")
- Native adapters: Django, Alembic, Rails, Laravel, EF Core, Knex, TypeORM, Sequelize, Liquibase (XML/YAML/JSON), Flyway Java-based (via capture).
- Capture mode (§13).
- `init` wizard, `doctor`.
- Framework-native fix suggestions.
- SARIF, GitHub annotations, Markdown, JUnit output.
- GitHub Action + GitLab template + pre-commit hook.
- All distribution channels (§22).

### v0.3 ("App-aware")
- Code usage scanner for Go, Python, Ruby, JS/TS, PHP, Java/Kotlin, C# (§14).
- Live DB mode with impact estimates (§15).
- Interactive TUI explorer (`-i`).
- Baselines.

### v1.0 ("Trustworthy")
- Full Postgres rule catalog, empirically lock-verified on PG 12–18.
- Real-world corpus false positive rate targets met (§23.5).
- Stable JSON schema, config schema, rule IDs (semver guarantees).
- Docs site complete.

### v2.x: MySQL/MariaDB. v3.x: SQL Server, SQLite, CockroachDB. (§10)

### Explicit non-goals (for now)
- Executing migrations (we are a reviewer, not a runner).
- Generating migrations from schema diffs (Atlas/ORMs already do this).
- Schema drift detection between environments.
- A hosted SaaS required for any core feature.

---

## 6. System architecture

```
                         ┌──────────────────────────────────────────────┐
  migrations on disk ──▶ │  1. DISCOVERY                                │
  git (base..HEAD)       │  detect project(s), framework(s), dialect,   │
  stdin                  │  changed migrations                          │
                         └───────────────────────┬──────────────────────┘
                                                 ▼
                         ┌──────────────────────────────────────────────┐
                         │  2. ADAPTERS  (framework → SQL + metadata)   │
                         │  raw SQL │ framework CLI │ capture (Docker)  │
                         │  output: []Migration{statements, txn mode,   │
                         │          source map}                         │
                         └───────────────────────┬──────────────────────┘
                                                 ▼
                         ┌──────────────────────────────────────────────┐
                         │  3. PARSE  (dialect parser → normalized IR)  │
                         └───────────────────────┬──────────────────────┘
                                                 ▼
    optional ──────────▶ ┌──────────────────────────────────────────────┐
    live DB stats        │  4. ANALYZE                                  │
    code usage index ──▶ │  context tracker (txn state, tables created  │
                         │  in this change set, PG version) → rules     │
                         └───────────────────────┬──────────────────────┘
                                                 ▼
                         ┌──────────────────────────────────────────────┐
                         │  5. POST-PROCESS                             │
                         │  suppressions, baseline, severity overrides, │
                         │  dedupe, sort, framework-native fixes        │
                         └───────────────────────┬──────────────────────┘
                                                 ▼
                         ┌──────────────────────────────────────────────┐
                         │  6. REPORT                                   │
                         │  pretty │ TUI │ json │ sarif │ github │ md │ │
                         │  junit                                       │
                         └──────────────────────────────────────────────┘
```

**Language:** Go (≥ 1.26.4). Chosen for single static-ish binary, fast startup, great concurrency, great terminal ecosystem (Charm).

### Key dependencies
| Purpose | Library |
|---|---|
| CLI framework | `spf13/cobra` |
| Styling / layout | `charmbracelet/lipgloss` |
| Interactive TUI | `charmbracelet/bubbletea` + `charmbracelet/bubbles` |
| Forms (init wizard) | `charmbracelet/huh` |
| Markdown rendering (`explain`) | built in (`internal/ui/markdown`); glamour dropped, see M2 notes |
| Terminal capability detection | `muesli/termenv` / `charmbracelet/colorprofile` |
| Postgres parser | `pganalyze/pg_query_go` (real Postgres parser, cgo) |
| MySQL parser (v2) | `pingcap/tidb/pkg/parser` (pure Go) |
| Code parsing | `tree-sitter/go-tree-sitter` + per-language grammars |
| DB driver (live mode) | `jackc/pgx/v5` |
| YAML config | `goccy/go-yaml` |
| JSON schema validation | `santhosh-tekuri/jsonschema` |
| Concurrency | `golang.org/x/sync/errgroup` |
| Docker (capture) | Docker Engine API client, or `testcontainers-go` |
| Release | `goreleaser` |
| Git | shell out to `git` CLI (faster and more compatible than go-git for diff/merge-base) |

**cgo note:** pg_query_go and tree-sitter need cgo. This affects cross-compilation (§22.2) but gives us the *real* Postgres parser, which is non-negotiable for correctness.

---

## 7. Repository layout

```
migrail/
├── cmd/migrail/                  # main.go: wires cobra root, nothing else
├── internal/
│   ├── cli/                      # one file per command: check.go, explain.go, init.go…
│   ├── config/                   # load/merge/validate .migrail.yaml, defaults, JSON schema
│   ├── discovery/                # project detection, framework markers, git diff, file walking
│   ├── gitx/                     # merge-base, changed files, blame-free "is new" detection
│   ├── adapters/
│   │   ├── adapter.go            # Adapter interface + registry
│   │   ├── sqlfiles/             # generic .sql dir
│   │   ├── golangmigrate/  goose/  atlasdir/  flyway/  prisma/  drizzle/  dbmate/  sqitch/
│   │   ├── django/  alembic/  rails/  laravel/  efcore/  knex/  typeorm/  sequelize/  liquibase/
│   │   └── capture/              # Docker/ephemeral DB statement capture
│   ├── dialect/
│   │   ├── dialect.go            # Dialect interface
│   │   ├── postgres/             # parser wrapper, IR builder, lock model, version matrix
│   │   └── mysql/                # (v2)
│   ├── ir/                       # normalized statement model (dialect-neutral)
│   ├── analyze/                  # context tracker, rule runner, scheduling
│   ├── rules/
│   │   ├── registry.go
│   │   ├── postgres/             # ms101_create_index_nonconcurrent.go + .md + _test.go …
│   │   └── common/               # dialect-agnostic rules (e.g. drop column vs code usage)
│   ├── fixes/                    # framework-native fix templates
│   ├── codescan/
│   │   ├── index.go              # symbol index: tables/columns → references
│   │   ├── golang/  python/  ruby/  jsts/  php/  jvm/  dotnet/
│   │   └── sqlstrings/           # SQL-in-string-literal extraction
│   ├── livedb/                   # read-only stats collection, impact estimation
│   ├── suppress/                 # inline comments, config ignores, baseline
│   ├── report/
│   │   ├── pretty/  tui/  json/  sarif/  github/  markdown/  junit/
│   └── ui/
│       ├── theme/                # color tokens, symbols, adaptive palettes
│       ├── components/           # codeframe, badge, tree, summary bar, spinner, table
│       └── term/                 # width, TTY, color profile, hyperlinks detection
├── pkg/migrail/                  # small public Go API (embed the engine in other tools)
├── schemas/                      # config.schema.json, report.schema.json (generated)
├── packaging/
│   ├── npm/  pypi/  rubygems/  composer/  homebrew/  scoop/  docker/  nfpm/
│   └── install.sh
├── action/                       # GitHub Action (action.yml, composite)
├── integrations/                 # gitlab-ci template, pre-commit hooks, bitbucket pipe
├── testdata/
│   ├── sql/                      # rule fixtures
│   ├── projects/                 # tiny real apps per framework (django-app, rails-app, …)
│   ├── golden/                   # terminal output snapshots
│   └── corpus/                   # scripts to fetch real OSS migrations (not committed)
├── tools/
│   ├── lockverify/               # empirical lock verification harness (§23.3)
│   ├── gendocs/                  # generates docs site pages + schemas from rule metadata
│   └── bench/
├── docs/                         # docs site source
├── .goreleaser.yaml
├── Makefile / Taskfile.yml
├── PLAN.md
├── CONTRIBUTING.md
└── LICENSE (Apache-2.0)
```

---

## 8. Core data model

```go
// ir package (dialect-neutral)

type Dialect string // "postgres", "mysql", …

type Project struct {
    Root       string
    Name       string
    Dialect    Dialect
    DBVersion  Version     // e.g. 16.0 (from config, live DB, or default)
    Frameworks []FrameworkRef
    Language   []string    // detected app languages for code scan
}

type Migration struct {
    ID          string       // stable: framework + version/name
    Version     string       // "20260917101500", "0042", …
    Name        string       // "add_status_to_orders"
    Framework   string       // "goose", "django", …
    SourcePath  string       // file the user edits
    Direction   Direction    // Up | Down
    TxMode      TxMode       // Transactional | NonTransactional | Unknown | ByStatements (resolved after parsing)
    Statements  []*Statement
    ChangeState ChangeState  // New | Modified | Unchanged (vs git base)
    Origin      Origin       // RawSQL | FrameworkCLI | Capture
}

type Statement struct {
    Index     int
    SQL       string         // exact text
    Node      any            // dialect AST (e.g. *pg_query.Node)
    Kind      StmtKind       // CreateIndex, AlterTableAddColumn, …
    Targets   []ObjectRef    // tables/indexes/types touched
    Columns   []ColumnRef
    Span      Span           // location in generated SQL
    SourceMap *SourceLoc     // location in user's file (e.g. Django operation line)
    InTx      bool           // resolved by context tracker
}

type Finding struct {
    RuleID     string        // "MR101"
    Slug       string        // "create-index-non-concurrent"
    Severity   Severity      // Error | Warning | Notice
    Confidence Confidence    // Definite | Likely | Possible
    Title      string        // one line, specific: `Index creation blocks writes on "orders"`
    Why        string        // 1–3 sentences
    Lock       *LockImpact   // mode, tables, blocks (reads/writes), rewrite bool
    Impact     *Impact       // from live DB: rows, size, est. duration range
    Evidence   []Evidence    // code references, related statements
    Fix        *Fix          // text + code in the user's framework
    Location   SourceLoc
    Statement  *Statement
    Suppressed *Suppression  // non-nil if suppressed (still reported in JSON)
    Fingerprint string       // stable hash for baselines (rule + migration ID + normalized stmt)
}

type Fix struct {
    Summary   string
    Framework string
    Steps     []FixStep     // multi-step expand/contract plans
}

type FixStep struct {
    Title string            // "Deploy 1: add nullable column"
    Lang  string            // "sql", "python", "ruby", "php", …
    Code  string
}
```

### Rule interface

```go
type RuleMeta struct {
    ID              string       // "MR101"  (never reused, never renumbered)
    Slug            string
    Title           string
    Category        Category     // Locking | Rewrite | Compatibility | Transaction | Data | BestPractice
    Dialects        []Dialect
    DefaultSeverity Severity
    DefaultEnabled  bool
    MinVersion      *Version     // rule applies from DB version
    MaxVersion      *Version     // rule no longer applies after
    Docs            string       // embedded markdown (//go:embed mr101.md)
    Tags            []string
}

type StatementRule interface {
    Meta() RuleMeta
    Check(ctx *analyze.Context, s *ir.Statement) []Finding
}

type MigrationRule interface {   // needs whole-migration view (ordering, tx)
    Meta() RuleMeta
    CheckMigration(ctx *analyze.Context, m *ir.Migration) []Finding
}
```

---

## 9. Analysis pipeline

### 9.1 Discovery
1. Resolve root: `--dir`, else walk up from CWD to find `.migrail.yaml` or VCS root.
2. Load config (§16); if absent, use auto-detection.
3. Detect projects (monorepo-aware; each can have its own framework/dialect):

| Marker | Framework |
|---|---|
| `**/*.up.sql` + `*.down.sql` pairs | golang-migrate |
| `-- +goose Up` in `.sql` | goose |
| `atlas.sum` | Atlas dir |
| `V<n>__*.sql`, `flyway.conf` / `flyway.toml` | Flyway |
| `prisma/schema.prisma` + `prisma/migrations/*/migration.sql` | Prisma |
| `drizzle.config.*` + `drizzle/*.sql` | Drizzle |
| `db/migrations/*.sql` with `-- migrate:up` | dbmate |
| `sqitch.plan` | sqitch |
| `manage.py` + `*/migrations/0*.py` | Django |
| `alembic.ini` | Alembic |
| `Gemfile` with `rails` + `db/migrate/*.rb` | Rails |
| `artisan` + `database/migrations/*.php` | Laravel |
| `*.csproj` referencing `Microsoft.EntityFrameworkCore` + `Migrations/` | EF Core |
| `package.json` dep `knex` / `typeorm` / `sequelize` | Knex / TypeORM / Sequelize |
| `db.changelog-master.*` | Liquibase |

4. Detect dialect: config → Prisma `provider` / Django `ENGINE` / `database.yml` adapter / connection strings in `.env.example` / SQL syntax hints → default `postgres` with a notice.
5. Detect DB version: config → live DB → `docker-compose.yml` image tag (e.g. `postgres:16`) → default = **oldest supported** (conservative) with a notice: `assuming PostgreSQL 12 (set db.version to get version-specific advice)`.

### 9.2 Change detection (git-aware)
Old migrations have already run; linting them is noise. Default behavior:
- If in a git repo: base = `--base` flag → `$GITHUB_BASE_REF` / `$CI_MERGE_REQUEST_TARGET_BRANCH_NAME` → `origin/HEAD` → `main`/`master`. Compute `git merge-base`.
- Check migrations **added or modified** since merge-base, plus uncommitted and untracked ones.
- **Modified** already-merged migrations → finding `MR401 edited-applied-migration` (warning).
- `--all` checks everything. Paths passed explicitly are always checked.
- Not a git repo → check all, with a notice.

### 9.3 Context tracker
Walks migrations in order (framework ordering) and statements in order, maintaining:
- **Transaction state:** explicit `BEGIN/COMMIT`, framework default (e.g. goose wraps unless `-- +goose NO TRANSACTION`; Django `atomic`; Rails `disable_ddl_transaction!`; Flyway per-script; Prisma none by default…).
- **New objects set:** tables/indexes created *in this change set*. Lock/rewrite rules are **suppressed for objects that are new** (they're empty and unused), which removes a huge class of false positives.
- **Lock accumulator:** locks held in the current transaction, to detect lock stacking across tables and long-held locks.
- **Timeouts set:** `SET lock_timeout`, `SET statement_timeout` (session or `SET LOCAL`).
- **Schema knowledge:** from live DB if connected, otherwise inferred from earlier migrations (best effort, e.g. column types for type-change rules).

### 9.4 Rule execution
- Rules are pure functions over (context snapshot, statement) → parallelizable per migration.
- Rules filtered by dialect, version range, enabled state, `--rule` / `--skip-rule`.
- A panicking rule is recovered, reported as an internal error for that rule, and never crashes the run.

### 9.5 Post-processing
Order: suppressions → baseline → severity overrides → dedupe (same rule, same statement) → framework-native fix rendering → sort (file order, then line).

---

## 10. Dialects & lock models

```go
type Dialect interface {
    Name() ir.Dialect
    Parse(sql string) ([]*ir.Statement, error)      // multi-statement, with spans
    SupportedVersions() []Version
    LockModel() LockModel                            // statement → lock mode(s), rewrite?
    Rules() []rules.Rule
    Normalize(sql string) string                     // for fingerprints
    LiveStats() livedb.Collector                     // optional
}
```

### 10.1 PostgreSQL (v1): supported 12 → 18
The lock model is a **versioned table** (data, not scattered `if`s), verified by the harness in §23.3. Excerpt:

| Statement | Lock | Blocks | Rewrite / scan | Version notes |
|---|---|---|---|---|
| `CREATE INDEX` | SHARE | writes | builds index | – |
| `CREATE INDEX CONCURRENTLY` | SHARE UPDATE EXCLUSIVE | other DDL/VACUUM | 2 scans; not allowed in tx | – |
| `DROP INDEX` | ACCESS EXCLUSIVE | everything | – | `CONCURRENTLY` available |
| `REINDEX` | ACCESS EXCLUSIVE on index + SHARE on table | reads using index / writes | rebuild | `CONCURRENTLY` PG12+ |
| `ADD COLUMN` (no default / non-volatile default) | ACCESS EXCLUSIVE (brief) | everything, briefly | none | PG11+ no rewrite for non-volatile default |
| `ADD COLUMN … DEFAULT <volatile>` (e.g. `gen_random_uuid()`, `clock_timestamp()`) | ACCESS EXCLUSIVE | everything | **rewrite** | – |
| `ADD COLUMN … GENERATED … STORED` / `serial` / identity | ACCESS EXCLUSIVE | everything | **rewrite** | – |
| `ALTER COLUMN TYPE` | ACCESS EXCLUSIVE | everything | **rewrite** + index rebuild (unless binary-coercible, e.g. `varchar(n)`→`text`, increasing `varchar` length) | – |
| `ALTER COLUMN SET NOT NULL` | ACCESS EXCLUSIVE | everything | full scan | PG12+ skips scan if a validated `CHECK (col IS NOT NULL)` exists |
| `ADD CONSTRAINT … FOREIGN KEY` | SHARE ROW EXCLUSIVE on both tables | writes on both | validation scan | use `NOT VALID` + `VALIDATE` |
| `ADD CONSTRAINT … CHECK` | ACCESS EXCLUSIVE | everything | validation scan | use `NOT VALID` + `VALIDATE` |
| `VALIDATE CONSTRAINT` | SHARE UPDATE EXCLUSIVE | other DDL only | scan | – |
| `ADD UNIQUE / PRIMARY KEY` | ACCESS EXCLUSIVE | everything | index build | use `CREATE UNIQUE INDEX CONCURRENTLY` + `USING INDEX` |
| `RENAME COLUMN / TABLE` | ACCESS EXCLUSIVE (brief) | everything, briefly | – | app compatibility risk |
| `DROP COLUMN` | ACCESS EXCLUSIVE (brief) | everything, briefly | – | app compatibility risk |
| `ALTER TYPE … ADD VALUE` | ACCESS EXCLUSIVE on type (brief) | – | – | pre-12: not allowed in tx; 12+: new value unusable in same tx |
| `CREATE TRIGGER` | SHARE ROW EXCLUSIVE | writes | – | – |
| `REFRESH MATERIALIZED VIEW` | ACCESS EXCLUSIVE | reads | recompute | `CONCURRENTLY` needs a unique index |
| `VACUUM FULL`, `CLUSTER` | ACCESS EXCLUSIVE | everything | **rewrite** | – |
| `SET LOGGED/UNLOGGED`, `SET TABLESPACE` | ACCESS EXCLUSIVE | everything | **rewrite** | – |
| `TRUNCATE` | ACCESS EXCLUSIVE | everything | – | data loss |

> Any row in this table that the lock harness cannot confirm on a given version is marked `unverified` and its rule downgrades by one severity level on that version.

### 10.2 MySQL / MariaDB (v2)
Different danger model: `ALGORITHM=INSTANT|INPLACE|COPY`, `LOCK=NONE|SHARED|EXCLUSIVE`, metadata lock (MDL) pile-ups, replication lag from large DDL, version-specific INSTANT support (8.0.12+, 8.0.29+ for more ops). Rules: require explicit `ALGORITHM`/`LOCK`, flag COPY-algorithm operations on large tables, recommend gh-ost/pt-online-schema-change for large tables, and flag `utf8` → `utf8mb4` conversions. Parser: TiDB parser (pure Go).

### 10.3 Later dialects
- **SQL Server:** online index operations (`ONLINE = ON`, edition-dependent), schema modification locks.
- **SQLite:** `ALTER TABLE` limits; the table-rebuild pattern that ORMs emit.
- **CockroachDB:** Postgres-compatible syntax (reuse parser) but online schema change semantics and different rules.

---

## 11. Rule catalog

**Rule ID scheme** (IDs are permanent, never reused):
- `MR1xx` Locking (blocks reads/writes)
- `MR2xx` Rewrites & long scans
- `MR3xx` App compatibility (breaks running code)
- `MR4xx` Transactions & operational safety
- `MR5xx` Data safety
- `MR6xx` Best practices (off by default unless noted)
- `MR9xx` Tool diagnostics (parse failures, unknown tx mode)

Severity: **error** (likely outage/breakage), **warning** (risky depending on size/traffic), **notice** (advice).
With live DB (§15), size-dependent rules auto-adjust: tiny tables downgrade, huge tables upgrade.

### 11.1 PostgreSQL rules

| ID | Slug | Sev | Ver | Triggers when | Safe alternative | Rel |
|---|---|---|---|---|---|---|
| MR101 | create-index-non-concurrent | error | all | `CREATE INDEX` without `CONCURRENTLY` on existing table | `CREATE INDEX CONCURRENTLY` outside tx | **v0.1** |
| MR102 | drop-index-non-concurrent | warning | all | `DROP INDEX` without `CONCURRENTLY` | `DROP INDEX CONCURRENTLY` outside tx | **v0.1** |
| MR103 | reindex-non-concurrent | warning | 12+ | `REINDEX` without `CONCURRENTLY` | `REINDEX … CONCURRENTLY` | v0.2 |
| MR104 | add-foreign-key-validating | error | all | `ADD FOREIGN KEY` without `NOT VALID` on existing table | `NOT VALID`, then `VALIDATE CONSTRAINT` in separate tx | **v0.1** |
| MR105 | add-check-validating | error | all | `ADD CHECK` without `NOT VALID` | same pattern | **v0.1** |
| MR106 | add-unique-constraint-blocking | error | all | `ADD UNIQUE` / `ADD PRIMARY KEY` building index inline | `CREATE UNIQUE INDEX CONCURRENTLY` + `ADD CONSTRAINT … USING INDEX` | **v0.1** |
| MR107 | set-not-null-scan | error | all | `SET NOT NULL` on existing column | PG12+: `CHECK (col IS NOT NULL) NOT VALID` → `VALIDATE` → `SET NOT NULL` → drop check | **v0.1** |
| MR108 | create-trigger-locks-writes | notice | all | `CREATE TRIGGER` on existing table | keep brief; set `lock_timeout` | v0.2 |
| MR109 | refresh-matview-blocking | warning | all | `REFRESH MATERIALIZED VIEW` without `CONCURRENTLY` | add unique index + `CONCURRENTLY` | v0.2 |
| MR110 | lock-stacking | warning | all | ≥ 2 existing tables take ≥ SHARE ROW EXCLUSIVE in one tx | split into separate migrations | v0.2 |
| MR111 | enum-add-value-in-tx | error/notice | <12 error, 12+ notice | `ALTER TYPE ADD VALUE` in tx | separate non-tx migration | v0.2 |
| MR201 | alter-column-type-rewrite | error | all | type change that isn't binary-coercible | expand/contract: new column, dual write, backfill, swap | **v0.1** |
| MR202 | add-column-volatile-default | error | all | `ADD COLUMN … DEFAULT <volatile fn>` | add nullable, backfill in batches, then set default | **v0.1** |
| MR203 | add-column-default-pre-11 | error | <11 | any default on `ADD COLUMN` | add column, set default separately, backfill | (only if <11 supported) |
| MR204 | add-stored-generated-or-serial | error | all | `ADD COLUMN` with `GENERATED STORED`, `serial`, identity | add nullable + sequence + backfill | v0.2 |
| MR205 | vacuum-full-or-cluster | error | all | `VACUUM FULL`, `CLUSTER` | `pg_repack` | **v0.1** |
| MR206 | set-logged-or-tablespace | error | all | `SET LOGGED/UNLOGGED`, `SET TABLESPACE` | plan maintenance window | v0.2 |
| MR207 | add-column-not-null-no-default | error | all | `ADD COLUMN … NOT NULL` without default on existing table | add with default, or nullable + backfill + MR107 pattern | **v0.1** |
| MR301 | drop-column-in-use | error | all | `DROP COLUMN` and code still references it (§14), or code removal is in the same deploy | expand/contract: remove usage → deploy → drop. Rails: `ignored_columns`; Django: `SeparateDatabaseAndState` | v0.3 (without code scan: warning in **v0.1**) |
| MR302 | rename-column | error | all | `RENAME COLUMN` on existing table | add new column, dual write, backfill, switch reads, drop old | **v0.1** |
| MR303 | rename-table | error | all | `RENAME TABLE` | create view with old name during transition, or expand/contract | **v0.1** |
| MR304 | drop-table-in-use | error | all | `DROP TABLE` referenced by code | remove code usage first | v0.3 (warning in **v0.1**) |
| MR305 | add-not-null-column-app-inserts | warning | all | new `NOT NULL` column without default while code inserts into table | add default or deploy code first | v0.3 |
| MR306 | narrowing-type-change | error | all | type narrowing (`bigint`→`int`, shorter `varchar`) | expand/contract | v0.2 |
| MR401 | edited-applied-migration | warning | all | migration already on base branch was modified | create a new migration | **v0.1** |
| MR402 | concurrent-in-transaction | error | all | `CONCURRENTLY` inside a transaction (will fail at runtime) | mark migration non-transactional (framework syntax) | **v0.1** |
| MR403 | missing-lock-timeout | warning | all | lock-taking DDL on existing table with no `lock_timeout` set | `SET lock_timeout = '5s'` (configurable) + retries | **v0.1** |
| MR404 | not-valid-validate-same-tx | error | all | `ADD … NOT VALID` then `VALIDATE` in same tx (lock is still held) | separate migrations/tx | **v0.1** |
| MR405 | mixed-ddl-and-backfill | warning | all | DDL + large `UPDATE`/`INSERT … SELECT` in same tx | separate migration, batched backfill | v0.2 |
| MR406 | unbatched-backfill | warning | all | `UPDATE` / `DELETE` on existing table without batching | batch by PK ranges, outside DDL tx | v0.2 |
| MR407 | failed-concurrent-index-leftover | notice | all | `CREATE INDEX CONCURRENTLY` without `IF NOT EXISTS` / cleanup plan | mention `INVALID` index cleanup | v0.2 |
| MR501 | truncate-table | error | all | `TRUNCATE` | confirm intent; suppress with reason | **v0.1** |
| MR502 | drop-table | warning | all | `DROP TABLE` (data loss) | backup / rename-then-drop-later | **v0.1** |
| MR503 | delete-or-update-without-where | error | all | `UPDATE`/`DELETE` with no `WHERE` | add predicate | **v0.1** |
| MR504 | lossy-type-change | error | all | `USING` casts that can truncate/round | explicit check | v0.2 |
| MR601 | prefer-bigint-pk | notice | all | new table PK is `int`/`serial` | `bigint` / `bigserial` / identity | v0.2 |
| MR602 | prefer-timestamptz | notice | all | `timestamp without time zone` | `timestamptz` | v0.2 |
| MR603 | prefer-text-over-varchar | off | all | `varchar(n)` | `text` + check | v0.2 |
| MR604 | prefer-jsonb | notice | all | `json` column | `jsonb` | v0.2 |
| MR605 | fk-without-index | notice | all | FK column without supporting index | add index concurrently | v0.2 |
| MR606 | prefer-identity | off | all | `serial` | `GENERATED ALWAYS AS IDENTITY` | v0.2 |
| MR901 | parse-error | error | – | SQL couldn't be parsed | show parser error with code frame | **v0.1** |
| MR902 | unknown-tx-mode | notice | – | adapter couldn't determine tx mode | set explicitly | v0.2 |
| MR903 | dynamic-sql | notice | – | `EXECUTE format(...)` / `DO` blocks: cannot analyze inner SQL | review manually | **v0.1** |

### 11.2 Rule docs format (embedded `mrXXX.md`)
Every rule doc has exactly these sections, rendered by `explain` and the docs site:

```
# MR101 · create-index-non-concurrent
## What it detects
## Why it's dangerous        (plain language, then precise lock details)
## Lock & impact             (table: lock mode, blocks, version notes)
## How to fix                (tabs per framework)
## When it's safe to ignore
## Examples                  (bad / good)
## References                (Postgres docs links)
```

### 11.3 Custom rules (v1.0+)
- **Policy rules in config** (no code): e.g. `forbid: { statements: [DropTable], tables: ["payments*"] }`, `max_table_rows_for_rewrite: 100000`.
- Plugin rules via WASM (post-1.0, §27).

---

## 12. Framework adapters

### 12.1 Interface

```go
type Adapter interface {
    Name() string                                     // "django"
    Detect(root string) (confidence float64, err error)
    Discover(ctx context.Context, p *ir.Project) ([]MigrationFile, error)   // cheap: list + order
    Extract(ctx context.Context, files []MigrationFile) ([]*ir.Migration, error) // → SQL
    TxMode(m *ir.Migration) ir.TxMode
    FixRenderer() fixes.Renderer                      // framework-native fixes
    Requirements() []Requirement                      // e.g. python + django importable
}
```

### 12.2 Extraction strategies (best available wins)
1. **Static (raw SQL):** read files directly. Fastest, no runtime needed.
2. **Framework CLI:** ask the framework to print SQL without running anything.
3. **Capture:** run migrations against a throwaway DB and record SQL (§13).

If strategy 2 needs a runtime that isn't available (e.g. no Python venv), migrail tells the user and offers capture mode or static fallback.

### 12.3 Per-framework details

| Framework | Strategy | How SQL is obtained | Tx mode detection | Source mapping |
|---|---|---|---|---|
| golang-migrate | static | `*.up.sql` | explicit `BEGIN` or none (driver runs multi-statement as-is) | exact line |
| goose (SQL) | static | `-- +goose Up` section; honors `StatementBegin/End` | tx unless `-- +goose NO TRANSACTION` | exact line |
| goose (Go funcs) | capture | – | from `AddMigrationNoTx` | file |
| Atlas dir | static | `.sql` files | `-- atlas:txmode none` directives | exact line |
| Flyway SQL | static | `V*__*.sql` | tx by default; non-tx when every statement must run outside a transaction (Flyway's Postgres auto-detection, `mixed=false`); per-script `executeInTransaction` config | exact line |
| Flyway Java | capture | – | – | file |
| Prisma | static | `migrations/*/migration.sql` | no implicit wrapping (per Prisma behavior; verify per version) | exact line |
| Drizzle | static | `drizzle/*.sql`, split on `--> statement-breakpoint` | – | exact line |
| dbmate | static | `-- migrate:up` | tx unless `transaction:false` | exact line |
| sqitch | static | `deploy/*.sql` | explicit `BEGIN`/`COMMIT` | exact line |
| Liquibase | static (SQL/formatted SQL) or CLI `update-sql` (XML/YAML/JSON) | `liquibase update-sql` | `runInTransaction` attr | changeset id → line |
| Django | CLI | `python manage.py sqlmigrate <app> <name>` per new migration | `atomic` attribute in Migration class (tree-sitter) | operation index → line via tree-sitter |
| Alembic | CLI | `alembic upgrade <prev>:<rev> --sql` (offline mode) | `transaction_per_migration` / env.py; `autocommit_block()` | op call → line |
| Rails | capture (primary), static AST (fallback) | ActiveRecord has no reliable dry run → capture; fallback: tree-sitter Ruby maps DSL (`add_index`, `add_column`, `change_column`, …) to IR directly | `disable_ddl_transaction!` | exact line |
| Laravel | CLI | `php artisan migrate --pretend` | `$withinTransaction` property | Schema builder call → line |
| EF Core | CLI | `dotnet ef migrations script <from> <to>` | `migrationBuilder.Sql(..., suppressTransaction: true)` | migration class → file |
| Knex | capture or CLI | Knex `toSQL()` via small runner script | `config.transaction = false` | file |
| TypeORM | static AST | `queryRunner.query("…")` string literals extracted with tree-sitter; builder API → capture | `transaction` option | exact line for literals |
| Sequelize | capture | – | – | file |

**Static DSL fallback (Rails, Laravel, Django):** a tree-sitter mapping of common DSL calls → IR (e.g. `add_index :orders, :status` → `CreateIndex{concurrently:false}`), so users without a working runtime still get the top rules. Marked `Confidence: Likely`.

### 12.4 Framework-native fixes
Every rule's fix is a template keyed by framework, with a generic SQL fallback. Example **MR101**:

- **SQL / goose:**
  ```sql
  -- +goose NO TRANSACTION
  -- +goose Up
  CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);
  ```
- **Rails:**
  ```ruby
  class AddIndexToOrdersStatus < ActiveRecord::Migration[7.2]
    disable_ddl_transaction!
    def change
      add_index :orders, :status, algorithm: :concurrently
    end
  end
  ```
- **Django:**
  ```python
  from django.contrib.postgres.operations import AddIndexConcurrently
  class Migration(migrations.Migration):
      atomic = False
      operations = [AddIndexConcurrently("order", models.Index(fields=["status"], name="idx_orders_status"))]
  ```
- **Laravel:**
  ```php
  public $withinTransaction = false;
  public function up(): void {
      DB::statement('CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status)');
  }
  ```

Templates use actual identifiers from the finding (table, column, index name).

---

## 13. Capture mode (universal adapter)

**Goal:** get the exact SQL from *any* framework, including custom ones, by observing what it sends to the database.

### Flow
```
migrail capture --framework rails -- bin/rails db:migrate
```
1. **Provision:** start `postgres:<db.version>` container (Docker/Podman) on a random port with
   `-c log_statement=all -c log_line_prefix='%m [%p] %x '`, tmpfs data dir, `fsync=off` for speed.
   Alternatively `--capture-db-url` for users without Docker (must be a disposable database. migrail refuses non-empty DBs unless `--force`).
2. **Baseline to base branch:** create a git worktree at merge-base in a temp dir and run the migrate command there against the DB (so only new migrations are captured with correct pre-existing schema). Cached by hash of base migration files (§21).
3. **Mark:** insert a log marker (`SELECT 'migrail:begin'`).
4. **Run new migrations:** run the user's command in the working tree with `DATABASE_URL` (and framework-specific env vars: `DATABASE_URL`, `DB_HOST/PORT/…`, `PG*`) pointing at the container.
5. **Collect & split:** parse the statement log; attribute statements to migrations using the framework's **bookkeeping writes** as delimiters:

| Framework | Bookkeeping table |
|---|---|
| Rails | `schema_migrations` |
| Django | `django_migrations` |
| Laravel | `migrations` |
| Alembic | `alembic_version` |
| Knex | `knex_migrations` |
| TypeORM | `migrations` (configurable) |
| Sequelize | `SequelizeMeta` |
| EF Core | `__EFMigrationsHistory` |
| Flyway | `flyway_schema_history` |
| goose | `goose_db_version` |
| golang-migrate | `schema_migrations` |
| generic | `--split-by-table <name>` or single migration |

6. **Filter noise:** drop introspection queries (`pg_catalog`, `information_schema`, `SHOW`, `SET search_path`, bookkeeping itself).
7. **Tx detection:** from logged `BEGIN`/`COMMIT` + xid in prefix, which is exact.
8. **Teardown:** remove container (always, including on Ctrl-C).

### UX
- Progress view: `Starting postgres:16 … ✓ 1.2s → Applying base (38 migrations) … ✓ cached → Capturing 2 new migrations … ✓ 0.8s`.
- Clear errors when the migrate command fails (show last 20 lines of its output).
- Config can store the capture command so `migrail check` uses capture automatically for that project.

---

## 14. Code usage scanner

**Goal:** know whether dropping/renaming a column or table breaks the app during deploy.

### 14.1 Symbol index
Built once per run (parallel, cached by file hash):

```
table "orders"
  column "status"
    ├─ definite  app/models/order.rb:12            (ActiveRecord attribute via schema)
    ├─ definite  internal/orders/model.go:18       (`db:"status"` tag)
    ├─ likely    internal/orders/repo.go:44        (raw SQL string: SELECT status FROM orders)
    └─ possible  web/src/api/orders.ts:9           (identifier `status` near "orders")
```

### 14.2 Per-language extractors (tree-sitter)

| Language | ORM/model sources | Raw SQL sources |
|---|---|---|
| Go | struct tags `db:`, `gorm:"column:"`, `bun:`, sqlc generated `*.sql.go` + `queries/*.sql`, ent schema | string literals passed to `Query/Exec/QueryRow/Raw/Select` |
| Python | Django models (`models.Model`, `db_column`, `db_table`), SQLAlchemy `Column`/`mapped_column`, `__tablename__` | `cursor.execute`, `text()`, `.raw()`, `.extra()` |
| Ruby | ActiveRecord models + `db/schema.rb`/`structure.sql`, `self.table_name`, `ignored_columns` | `find_by_sql`, `where("…")`, `execute`, `select("…")` |
| JS/TS | Prisma schema (`@map`, `@@map`), TypeORM `@Column({name})`, Sequelize `define`, Drizzle `pgTable`, Knex/Kysely query builder calls | template literals in `query`, `sql\`\``, `$queryRaw` |
| PHP | Eloquent `$table`, `$fillable`, `$casts`, attributes | `DB::select/statement/raw`, `whereRaw` |
| Java/Kotlin | JPA `@Table`, `@Column(name)`, naming strategy (snake_case default), jOOQ generated classes | `@Query(nativeQuery)`, JdbcTemplate strings |
| C# | EF `[Table]`, `[Column]`, `ToTable`, `HasColumnName`, conventions | `FromSqlRaw`, Dapper strings |

- **SQL string extraction:** string literals that look like SQL are parsed by the dialect parser to find real column/table references (`Likely`). Unparseable ones fall back to token matching (`Possible`).
- **Naming conventions:** snake_case ↔ camelCase mapping per ORM defaults (e.g. JPA physical naming, Prisma exact).

### 14.3 Deploy-compatibility logic (MR301/MR302/MR304)
Rolling deploys run **old code** while the migration runs. So:

| Situation | Result |
|---|---|
| Column dropped, **current code still references** it | **error**: will break after deploy |
| Column dropped, reference **removed in the same diff** (git base still has it) | **error**: old instances break during deploy; ship code removal first |
| Column dropped, no references in base or HEAD | ok (MR502-style notice only) |
| Rails: column dropped without being in `ignored_columns` in base | error: AR schema cache will break old instances |
| Rename with any reference in base | error (always expand/contract) |

`Possible`-confidence-only evidence produces **warning**, not error. Evidence is shown in output with clickable file:line links.

### 14.4 Controls
`codescan.enabled`, `codescan.paths`, `codescan.exclude` (defaults respect `.gitignore`, and skip `vendor/`, `node_modules/`, `dist/`, `build/`, migrations dirs, and test fixtures optionally).

---

## 15. Live database mode

**Opt-in only:** `--db <url>` or `MIGRAIL_DATABASE_URL`, or `db.url_env` in config (never store URLs in config).

### Safety
- Connection opened with `default_transaction_read_only=on`, `statement_timeout=5s`, `application_name=migrail`.
- Only catalog/stat queries. Never touches user tables' data.
- Credentials never logged or printed (URL redacted in all output, including `--debug`).

### Collected (Postgres)
| Data | Source |
|---|---|
| Server version | `SHOW server_version_num` |
| Row estimate, table/index/toast size | `pg_class.reltuples`, `pg_total_relation_size`, `pg_indexes_size` |
| Write activity | `pg_stat_user_tables` (`n_tup_ins/upd/del` deltas over 2s sample), `n_live_tup` |
| Existing indexes/constraints/column types | `pg_index`, `pg_constraint`, `pg_attribute` |
| Long-running transactions right now | `pg_stat_activity` (informational in `doctor`) |
| Whether a column has a validated NOT NULL check | `pg_constraint` (sharpens MR107) |

### Impact estimation
- Rewrite/scan/index build duration: `size / throughput`, with throughput calibrated by a benchmark table in `tools/bench` (per operation class), shown as a **range** (e.g. `~4–9 min`) and always labeled *estimate*.
- Severity adjustment thresholds (configurable):
  - `< 10k rows` and `< 10 MB`: downgrade one level ("table is small").
  - `> 10M rows` or `> 10 GB`: upgrade warning → error.
- Missing objects in live DB (e.g. table created by an earlier unapplied migration) → treated as new.

---

## 16. Configuration

File: `.migrail.yaml` (also `.migrail.yml`, `migrail.toml`?, no: **YAML only**, one format). Validated against `schemas/config.schema.json` (published so editors autocomplete).

```yaml
# yaml-language-server: $schema=https://<docs-domain>/schema/config.json
version: 1

db:
  dialect: postgres            # auto-detected if omitted
  version: "16"                # strongly recommended
  url_env: STAGING_DATABASE_URL # optional live mode (env var name, never the URL)

projects:                      # omit for single-project auto-detect
  - name: api
    path: services/api
    framework: goose
    migrations: db/migrations
  - name: web
    path: apps/web
    framework: django
    python: .venv/bin/python   # runtime hint for CLI extraction
  - name: billing
    path: services/billing
    framework: rails
    capture:
      command: bin/rails db:migrate
      env: { RAILS_ENV: test }

git:
  base: origin/main            # auto-detected if omitted
  check: changed               # changed | all

rules:
  MR403:
    lock_timeout_max: 10s
  MR601: notice                # severity override shorthand
  MR603: off
  MR110:
    severity: error

policy:
  fail_on: error               # error | warning | notice | never
  require_ignore_reason: true
  protected_tables: ["payments", "ledger_*"]   # any lock-taking op → error
  max_rows_for_rewrite: 100000                 # live mode

thresholds:
  small_table_rows: 10000
  large_table_rows: 10000000

codescan:
  enabled: true
  paths: ["."]
  exclude: ["**/testdata/**"]

ignore:
  - path: "db/migrations/2019*"          # legacy
  - rule: MR602
    path: "legacy/**"
    reason: "legacy schema, tracked in JIRA-123"

output:
  format: pretty               # pretty | json | sarif | github | markdown | junit
  theme: auto                  # auto | dark | light | mono
  compact: false
  hyperlinks: auto
```

**Precedence:** CLI flags > env vars (`MIGRAIL_*`) > `.migrail.yaml` > user config (`~/.config/migrail/config.yaml`, UI prefs only) > defaults.

Invalid config → exit code 2 with a code frame pointing into the YAML and a "did you mean" suggestion.

---

## 17. Suppressions & baselines

### 17.1 Inline suppressions
```sql
-- migrail:ignore MR101 reason="orders is empty in all environments"
CREATE INDEX idx_orders_status ON orders (status);
```
- Applies to the **next statement**. `-- migrail:ignore-file MR101 reason="…"` applies to the file.
- Comment syntax adapts: `# migrail:ignore` (Python/Ruby), `// migrail:ignore` (PHP/JS/Go/C#/Java).
- `reason` required when `policy.require_ignore_reason: true` (default). Missing reason → `MR904 ignore-without-reason`.
- Unused suppressions → `MR905 unused-ignore` (notice) so stale ignores get cleaned.
- Suppressed findings are hidden in pretty output (count shown in summary), present in JSON with `suppressed`.

### 17.2 Baseline
- `migrail baseline` writes `.migrail-baseline.json` with finding fingerprints from `--all`.
- Subsequent runs hide baselined findings. Useful when enabling `--all` on legacy repos.
- Fingerprint = hash(rule ID + migration ID + normalized statement), stable across whitespace/comment edits.

---

## 18. CLI specification

### 18.1 Commands

| Command | Purpose |
|---|---|
| `migrail` | alias for `migrail check` |
| `migrail check [paths…\|-]` | analyze migrations |
| `migrail explain <rule>` | rich rule docs (`MR101` or slug), framework tabs chosen from detected project |
| `migrail rules` | table of all rules: ID, slug, severity, category, enabled, versions. `--json` |
| `migrail init` | interactive setup wizard (§19.8) |
| `migrail doctor` | environment diagnostics: git, docker, runtimes, config validity, DB connectivity |
| `migrail capture -- <cmd>` | run capture mode (§13) and analyze |
| `migrail baseline` | create/update baseline |
| `migrail version` | version, commit, build date, parser version, supported dialects |
| `migrail completion <shell>` | bash, zsh, fish, powershell |
| `migrail schema <config\|report>` | print JSON schemas |

### 18.2 `check` flags
```
  -d, --dir <path>             project root (default: auto)
      --all                    check all migrations, not only changed
      --base <ref>             git base (default: auto)
      --framework <name>       force adapter
      --dialect <name>         force dialect
      --db-version <ver>       e.g. 16
      --db <url>               enable live mode (or MIGRAIL_DATABASE_URL)
      --no-codescan            disable code usage scanning
  -r, --rule <id>…             only these rules
      --skip-rule <id>…        skip rules
      --fail-on <level>        error|warning|notice|never
  -f, --format <fmt>           pretty|json|sarif|github|markdown|junit (repeatable with -o)
  -o, --output <file>          write format to file (e.g. -f sarif -o out.sarif)
  -i, --interactive            open TUI explorer
      --ci                     CI mode: no TTY features, auto-detect CI provider formats
      --compact                one line per finding
      --color <when>           auto|always|never
      --theme <name>           auto|dark|light|mono
      --no-hyperlinks
      --include-down           also analyze down migrations
  -q, --quiet                  only print findings and summary errors
  -v, --verbose / --debug      timings per phase, adapter commands, cache hits
```

### 18.3 Exit codes
| Code | Meaning |
|---|---|
| 0 | no findings at or above `fail_on` |
| 1 | findings at or above `fail_on` |
| 2 | usage or config error |
| 3 | adapter/extraction failure (e.g. `manage.py sqlmigrate` failed) |
| 4 | internal error (bug; prints report link) |
| 130 | interrupted |

### 18.4 Streams
- Findings/report → **stdout**. Progress, spinners, notices → **stderr**. So `migrail -f json > report.json` stays clean.

### 18.5 CI auto-detection (`--ci` or env `CI=true`)
- GitHub Actions → pretty (no color unless forced) + `github` annotations + job summary markdown written to `$GITHUB_STEP_SUMMARY`.
- GitLab → pretty + `codequality` JSON if `--output` given.
- Everything else → pretty plain.

---

## 19. Terminal design system

The terminal UI is a product feature. It should look like a modern compiler (Rust, Elm) crossed with Charm tools: calm, precise, color with meaning.

### 19.1 Design principles
1. **Hierarchy through weight and color, not boxes.** Minimal borders; whitespace does the grouping.
2. **Color carries meaning, never decoration.** Each color token maps to one semantic role.
3. **Scannable in 2 seconds:** severity symbol → rule → title is always the first line.
4. **Degrades perfectly:** truecolor → 256 → 16 → no color → no Unicode, all readable.
5. **No flicker, no layout jumps.** Progress on stderr only, cleared cleanly.
6. **Respect the user:** `NO_COLOR`, `CLICOLOR_FORCE`, `TERM=dumb`, narrow terminals, screen readers (mono theme).

### 19.2 Color tokens

| Token | Role | Dark (hex) | Light (hex) | 16-color fallback |
|---|---|---|---|---|
| `fg` | body text | `#E6E6EA` | `#1F2328` | default |
| `fg.muted` | secondary text, gutters | `#8B8D98` | `#6E7781` | bright black |
| `fg.subtle` | borders, rules | `#3A3C45` | `#D0D7DE` | bright black |
| `accent` | brand, headings, links | `#7C8CFF` | `#4F5BD5` | blue |
| `error` | error severity | `#FF6B81` | `#CF222E` | red |
| `warning` | warning severity | `#F5B95C` | `#9A6700` | yellow |
| `notice` | notice severity | `#6CC4FF` | `#0969DA` | cyan |
| `success` | clean run, "fix" label | `#5FD69B` | `#1A7F37` | green |
| `code.keyword` | SQL keywords in code frames | `#C792EA` | `#8250DF` | magenta |
| `code.string` | string literals | `#A5D6A7` | `#0A3069` | green |
| `code.ident` | identifiers | `#E6E6EA` | `#1F2328` | default |
| `highlight.bg` | underline/caret span | `#FF6B8126` (15% error) | `#CF222E1A` | reverse |

Background detection: `lipgloss.HasDarkBackground()` with a 100 ms timeout; if unknown → dark palette. `--theme` overrides.

### 19.3 Symbols

| Meaning | Unicode | ASCII fallback |
|---|---|---|
| error | `✖` | `x` |
| warning | `▲` | `!` |
| notice | `●` | `i` |
| success | `✔` | `ok` |
| suppressed | `◌` | `-` |
| file header marker | `▌` | `>` |
| artwork rail | `═╪═` | `=+=` |
| tree branches | `├─ │ ╰─ ╭─` | `\|- \| \`- ,-` |
| caret underline | `━━━━` (colored) | `^^^^` |
| spinner | braille dots `⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏` | `- \ \| /` |

Unicode fallback when `LANG`/`LC_ALL` lacks UTF-8, on legacy Windows console, or `--ascii`.

### 19.4 Typography & layout
- **Bold:** severity label, rule ID, file names. **Dim:** gutters, metadata, timings. **Italic:** avoided (inconsistent terminal support). **Underline:** only hyperlinks when OSC 8 is unsupported.
- **Width:** content width = `min(terminal width, 100)`; minimum supported 60. Below 60 → compact layout automatically.
- **Indentation:** 2 spaces per level; code frame gutter right-aligned to max line number width.
- **Wrapping:** prose wraps at content width with hanging indent; code is never wrapped (truncated with `…` and dim `→` marker if wider).
- **Hyperlinks:** OSC 8 links on file paths (`file://…#L4` or editor scheme via `output.editor_link: vscode|idea|zed|cursor|none`) and rule IDs → docs URL. Auto-detected support (iTerm2, WezTerm, Kitty, Windows Terminal, VS Code, GNOME Terminal/VTE).
- **Blank lines:** exactly one between findings, one before summary; never two.

### 19.5 Screens (exact layouts)

**A) Clean run** (the most common output: one line)
```
✔ 3 migrations checked · no issues · 42ms
```

**B) Findings (default, pretty)**
```
migrail 0.3.0 · postgres 16 · goose · 3 changed migrations vs origin/main

▌ db/migrations/20260917101500_add_orders_status.sql

  ✖ error MR101 Index creation blocks writes on "orders"

     ╭─ db/migrations/20260917101500_add_orders_status.sql:6:1
   5 │ ALTER TABLE orders ADD COLUMN status text;
   6 │ CREATE INDEX idx_orders_status ON orders (status);
     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │
     ├─ lock    SHARE on orders · blocks INSERT, UPDATE, DELETE
     ├─ impact  41.2M rows · 18.4 GB · est. 3–7 min of blocked writes   (live)
     ├─ why     Postgres holds this lock until the whole index is built.
     ╰─ fix     Build the index concurrently, outside a transaction:

                -- +goose NO TRANSACTION
                -- +goose Up
                CREATE INDEX CONCURRENTLY idx_orders_status ON orders (status);

  ▲ warning MR403 No lock_timeout before locking DDL

     ╭─ db/migrations/20260917101500_add_orders_status.sql:5:1
   5 │ ALTER TABLE orders ADD COLUMN status text;
     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
     │
     ├─ why     If a long query holds orders, this ALTER waits and every query behind it queues.
     ╰─ fix     Add at the top of the migration:

                SET lock_timeout = '5s';

▌ db/migrations/20260917102000_rename_user_email.sql

  ✖ error MR302 Renaming column "users.email" breaks running code

     ╭─ db/migrations/20260917102000_rename_user_email.sql:2:1
   2 │ ALTER TABLE users RENAME COLUMN email TO email_address;
     │                                 ━━━━━
     │
     ├─ used by  internal/users/model.go:14        `db:"email"`            definite
     │           internal/auth/repo.go:61          SELECT id, email FROM…  likely
     │           + 3 more (migrail check -v)
     ├─ why      During a rolling deploy old instances still query "email".
     ╰─ fix      Expand/contract in 3 deploys:
                 1  Add email_address, write to both columns, backfill
                 2  Read from email_address, stop writing email
                 3  Drop email
                 migrail explain MR302 for code

────────────────────────────────────────────────────────────────────────────────
  ✖ 2 errors  ▲ 1 warning  ● 0 notices  ◌ 1 ignored        3 files · 14 stmts · 118ms
  Failing: 2 findings at or above "error"
```

Details:
- Header line is `fg.muted` except the version number (`accent`). Printed only when there are findings.
- `▌` file marker in `accent`, path in bold `fg`, framework right-aligned in `fg.muted` (omitted if too narrow).
- Severity word colored + bold, rule ID bold `fg`, title `fg`. Table/column names in titles are wrapped in `"…"` and rendered in `accent`.
- Code frame shows the target line plus **1 line of context before** (max 2); SQL is syntax-highlighted with the dialect tokenizer.
- Underline spans exactly the relevant node (whole statement for MR101, column name for MR302) in the severity color.
- Labels (`lock`, `impact`, `why`, `fix`, `used by`) are `fg.muted`, left-aligned in a fixed 8-char column. `fix` label in `success`.
- `(live)` tag in `notice` color marks data from live DB; `est.` always shown for estimates.
- Summary rule line `─` in `fg.subtle`, full content width. Zero counts rendered in `fg.muted`.

**C) Compact (`--compact`, or width < 60, or `-q`)**
```
✖ MR101 db/migrations/20260917101500_add_orders_status.sql:6  Index creation blocks writes on "orders"
▲ MR403 db/migrations/20260917101500_add_orders_status.sql:5  No lock_timeout before locking DDL
✖ MR302 db/migrations/20260917102000_rename_user_email.sql:2  Renaming column "users.email" breaks running code
2 errors · 1 warning · 118ms
```
(`path:line` format is clickable in most editors/terminals.)

**D) Progress (stderr, only if the run takes > 150 ms and stderr is a TTY)**
```
⠹ Extracting SQL  django · 2 migrations  (python manage.py sqlmigrate)
```
Phases shown: `Discovering` → `Extracting SQL` → `Scanning code` → `Querying database` → `Analyzing`. Single line, rewritten in place, erased before results print. With `-v`, each completed phase is kept with its timing:
```
✔ Discovered        3 migrations · goose            4ms
✔ Extracted SQL     static                          2ms
✔ Scanned code      1,284 files · 9 languages      61ms
✔ Analyzed          14 statements · 43 rules        3ms
```

**E) Parse error (MR901)**
```
  ✖ error MR901 Could not parse SQL

     ╭─ db/migrations/20260917_x.sql:3:23
   3 │ ALTER TABLE orders ADD COLUM status text;
     │                        ━━━━━ syntax error at or near "COLUM"
     │
     ╰─ note    Other statements in this file were still analyzed.
```

**F) Adapter failure (exit 3)**
```
✖ Couldn't extract SQL for Django migrations

  Command   .venv/bin/python manage.py sqlmigrate orders 0042
  Exit      1
  Output    (last 6 lines)
            django.core.exceptions.ImproperlyConfigured: SECRET_KEY must not be empty

  Try
    • set env vars needed by settings: migrail check --env DJANGO_SETTINGS_MODULE=app.settings.ci
    • use capture mode:                migrail capture -- python manage.py migrate
    • run diagnostics:                 migrail doctor
```

**G) `migrail rules`**
```
  ID     Rule                              Severity  Category       Postgres
  MR101  create-index-non-concurrent       error     locking        all
  MR102  drop-index-non-concurrent         warning   locking        all
  MR107  set-not-null-scan                 error     locking        all  (12+ fix)
  MR603  prefer-text-over-varchar          off       best-practice  all
  …
  43 rules · 38 enabled · run migrail explain <ID>
```
Header row bold `fg.muted`; severity column colored; `off` rows fully dimmed.

**H) `migrail explain MR101`**: Glamour-rendered markdown using a custom migrail Glamour style matching tokens; framework section auto-selects the detected framework first, others listed after; piped to `$PAGER` when output exceeds terminal height (disable with `--no-pager`).

**J) Welcome artwork** (`migrail --help`, `migrail init`, `migrail version`)
```
 ███╗   ███╗██╗ ██████╗ ██████╗  █████╗ ██╗██╗
 ████╗ ████║██║██╔════╝ ██╔══██╗██╔══██╗██║██║
 ██╔████╔██║██║██║  ███╗██████╔╝███████║██║██║
 ██║╚██╔╝██║██║██║   ██║██╔══██╗██╔══██║██║██║
 ██║ ╚═╝ ██║██║╚██████╔╝██║  ██║██║  ██║██║███████╗
 ╚═╝     ╚═╝╚═╝ ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚═╝╚══════╝
═╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═══╪═
 catch dangerous migrations
 before they reach production
```
- Shown only when stdout is a TTY, the format is `pretty`, and none of `--quiet`, `--ci`, `CI=true` apply. `migrail version` without a TTY prints only its plain lines, so scripts can parse it.
- Wordmark: six rows, 51 columns, sized to fit the 60-column minimum. Solid blocks use a horizontal gradient from `accent` to `notice`, bold. Truecolor interpolates per column; 256 colors uses the nearest palette entries; 16 colors uses `accent` only. Shadow edges (`╗║╔╝╚═`) use `fg.subtle`.
- Rail line `═` in `fg.subtle`, ties `╪` in `fg.muted`, as wide as the wordmark. Tagline in `fg.muted`.
- One blank line after the artwork, then the screen's normal content.
- `NO_COLOR` / `--theme mono`: same art, no color. ASCII fallback (§19.3): solid blocks become `#`, shadow edges are dropped, rail becomes `=+===+…`.
- Width < 60: replaced by a single bold `migrail` line in `accent`.
- The art is static string data. It adds no measurable time to `migrail version` (§21 budget: < 15 ms).

**I) `migrail doctor`**
```
migrail doctor

  Environment
  ✔ git            2.46.0 · repo root /Users/me/api · base origin/main
  ✔ docker         27.1 · daemon reachable            (needed for capture mode)
  ▲ python         .venv not found                    set projects[].python
  ✔ config         .migrail.yaml valid

  Projects
  ✔ api            goose · postgres 16 · 214 migrations · db/migrations
  ▲ web            django · postgres (version unknown)  set db.version

  Database (MIGRAIL_DATABASE_URL)
  ✔ connected      PostgreSQL 16.4 · read-only session
  ● note           2 transactions open > 5 min right now

  2 warnings
```

### 19.6 Interactive explorer (`-i`), Bubble Tea
```
╭ migrail ───────────────────────────────────────────────────────────────────────╮
│ Findings (3)            filter: all ▾   │ ✖ MR101 Index creation blocks writes  │
│                                         │ on "orders"                           │
│ ▌ 20260917101500_add_orders_status      │                                       │
│ › ✖ MR101 Index blocks writes  :6       │   6 │ CREATE INDEX idx_orders_status  │
│   ▲ MR403 No lock_timeout      :5       │     │ ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━  │
│ ▌ 20260917102000_rename_user_email      │                                       │
│   ✖ MR302 Rename breaks code   :2       │ lock   SHARE · blocks writes          │
│                                         │ impact 41.2M rows · est. 3–7 min      │
│                                         │ fix    CREATE INDEX CONCURRENTLY …    │
│                                         │                                       │
├─────────────────────────────────────────┴───────────────────────────────────────┤
│ ↑↓ move  ⏎ details  o open in editor  c copy fix  x ignore  / filter  ? help  q  │
╰─────────────────────────────────────────────────────────────────────────────────╯
```
- Layout: 40/60 split ≥ 100 cols; stacked (list above detail) < 100 cols; min 60×15, otherwise falls back to pretty output.
- Keys: `↑↓/jk` move · `⏎/tab` focus detail · `o` open `$EDITOR` at line (`code -g`, `idea --line`, `vim +N`, `zed file:N`) · `c` copy fix to clipboard (OSC 52, falls back to system clipboard) · `x` add suppression (prompts for reason via `huh` input, writes comment into file, re-runs analysis) · `e` explain rule in-pane · `/` fuzzy filter · `1/2/3` toggle severities · `r` re-run · `?` help overlay · `q/esc` quit.
- Renders at ≤ 16 ms per frame; findings list virtualized for 10k+ items.
- Alt-screen, restores terminal on exit/panic.

### 19.7 Motion
- Spinner at 80 ms/frame; appears only after 150 ms (no flashes on fast runs).
- No animations in output after completion. TUI selection moves instantly (no easing).

### 19.8 `migrail init` wizard (huh forms)
```
  migrail init

  Detected
  ✔ goose migrations in db/migrations (214)
  ✔ postgres (from docker-compose.yml: postgres:16)
  ✔ Go app code (codescan ready)

  ? PostgreSQL version in production   › 16
  ? Fail CI on                         › errors only
  ? Require a reason for ignores       › yes
  ? Add GitHub Action workflow         › yes
  ? Adopt on existing migrations       › only check new migrations (recommended)

  Created .migrail.yaml
  Created .github/workflows/migrail.yml

  Next: migrail            check your changes now
        migrail explain    learn any rule
```
Non-interactive: `migrail init --yes` accepts detected defaults.

### 19.9 Accessibility
- `--theme mono`: no color, symbols + words carry all meaning (severity word always printed next to symbol).
- No information conveyed by color only.
- Screen reader friendly compact format (`--compact --ascii`).
- Honors `NO_COLOR` (no color), `FORCE_COLOR`/`CLICOLOR_FORCE` (force), `TERM=dumb` (plain).

### 19.10 Golden tests for design
Every screen above is a golden snapshot at widths **60, 80, 120** × profiles **truecolor, ansi16, no-color, ascii** (see §23.2). Design changes require updating goldens in the PR, so reviewers see the visual diff.

---

## 20. Output formats & CI integrations

### 20.1 Formats
| Format | Use | Notes |
|---|---|---|
| `pretty` | humans | §19 |
| `json` | tooling | stable schema `schemas/report.schema.json`, versioned `"schema": 1`; includes suppressed findings, timings, detected project info |
| `sarif` | GitHub code scanning, IDEs | SARIF 2.1.0; rules with help markdown; fingerprints for dedupe |
| `github` | Actions annotations | `::error file=…,line=…,title=MR101::…` |
| `markdown` | PR comments, job summary | collapsible `<details>` per finding, fix code blocks |
| `junit` | generic CI test reports | one testcase per migration |
| `gitlab` | GitLab Code Quality | codequality JSON |

### 20.2 GitHub Action (`action/`)
```yaml
- uses: <owner>/migrail-action@v1
  with:
    version: latest          # or pinned
    fail-on: error
    comment: true            # sticky PR comment (updated in place, deleted when clean)
    sarif: true              # upload to code scanning
    db-url: ${{ secrets.STAGING_DB_URL }}  # optional live mode
```
- Downloads the binary with checksum verification (cached across runs).
- PR comment: summary table + findings as collapsible sections + "all clear ✔" edit when fixed.
- Only needs `pull-requests: write` for comments; works read-only otherwise.

### 20.3 Other integrations
- **GitLab CI** template (`integrations/gitlab/migrail.gitlab-ci.yml`), code quality report.
- **pre-commit** hook (`.pre-commit-hooks.yaml`) + **lefthook**/**husky** snippets.
- **Bitbucket Pipelines**, **CircleCI orb**, **Azure Pipelines** snippets in docs.

---

## 21. Performance

### 21.1 Budgets (enforced by CI benchmarks, reference machine: GitHub `ubuntu-latest` 4 vCPU)

| Scenario | Budget |
|---|---|
| `migrail version` startup | < 15 ms |
| Clean run, 3 changed SQL migrations, codescan on 2k-file repo (warm cache) | < 150 ms |
| Same, cold cache | < 500 ms |
| `--all` on 5,000 SQL migrations | < 1.5 s |
| Codescan 50k-file monorepo, cold | < 4 s; warm < 400 ms |
| Peak RSS for above | < 300 MB |
| TUI frame | < 16 ms |
| Binary size (compressed download) | < 25 MB |

### 21.2 Techniques
- **Do less:** git-aware changed-only default; codescan only runs if a rule needs it (drop/rename present).
- **Parallelism:** errgroup worker pools (GOMAXPROCS) for file reading, parsing, tree-sitter parsing, rule execution per migration.
- **Content-addressed cache** at `$XDG_CACHE_HOME/migrail/` (`~/Library/Caches/migrail` on macOS):
  - parsed IR by `sha256(sql + parser version)`
  - codescan symbol index per file by `sha256(content + extractor version)`
  - framework CLI extraction output by `hash(migration files + lockfile + framework version)`
  - capture baseline DB dump by `hash(base migrations)` (pg_dump schema-only, restored in ms)
  - cache is size-capped (default 500 MB, LRU) and `--no-cache` bypasses.
- **Fast file walking:** gitignore-aware parallel walker, skip binary files by extension + first bytes.
- **Lazy init:** tree-sitter grammars loaded only for languages present; Glamour/Bubble Tea only imported on commands that use them (keep root package init light).
- **Batch framework CLI calls:** e.g. one Python process running a tiny script that calls `sqlmigrate` for all new migrations, instead of N processes (Django startup is slow).
- **Streaming render:** pretty output prints per file as soon as that file's analysis is done (ordering preserved by a sequencer).
- **Profiling hooks:** `--debug-profile cpu|mem` writes pprof files.

---

## 22. Distribution

### 22.1 Channels
| Channel | Install | Mechanism |
|---|---|---|
| Homebrew | `brew install migrail` | tap first (`<owner>/homebrew-tap`), homebrew-core once popular |
| npm | `npx migrail` / `npm i -D migrail` | esbuild-style: main package + `optionalDependencies` per platform (`migrail-darwin-arm64`, …); **no postinstall downloads** |
| PyPI | `pipx install migrail` / `uv tool install migrail` | platform wheels bundling binary (ruff-style) |
| RubyGems | `gem install migrail` | platform gems bundling binary |
| Packagist | `composer require --dev <owner>/migrail` | package with per-platform binaries in dist or verified download on first run |
| NuGet | `dotnet tool install -g migrail` | .NET tool package wrapping binaries per RID |
| Maven/Gradle | Gradle plugin `id("<reverse-domain>.migrail")` (v1.x) | downloads verified binary |
| Go | `go install github.com/<owner>/migrail/cmd/migrail@latest` | requires cgo toolchain; documented |
| Docker | `docker run --rm -v $PWD:/src ghcr.io/<owner>/migrail` | distroless/static image |
| Scoop / winget | `scoop install migrail` | manifests |
| Linux packages | `.deb`, `.rpm`, `.apk` | nfpm via goreleaser |
| Script | `curl -fsSL https://<docs-domain>/install.sh \| sh` | detects OS/arch, verifies checksum; `--uninstall` removes the binary and points Homebrew installs to `brew uninstall` |
| GitHub Action | `<owner>/migrail-action@v1` | §20.2 |

Every wrapper package version == binary version. A single release pipeline publishes all channels.

### 22.2 Build matrix (cgo)
Targets: `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64` (both **musl static** for portability), `windows/amd64`, `windows/arm64`.
- Approach: goreleaser with **zig cc** as C cross-compiler on one Linux runner for `linux/*` (musl, static) and `windows/*`. **darwin builds run on a macOS runner with native clang.** Checked on 2026-09-17 with zig 0.14.1 and a probe binary that links pg_query_go v6: Linux and Windows targets build and run, but darwin fails to link because zig ships no macOS SDK (`libresolv`, `CoreFoundation`).
- goreleaser OSS can't merge builds from several runners (`--split`/`--merge` is Pro). CI builds each group with `goreleaser build --id …` to catch breakage early.
- Releases run on one macOS runner (`.github/workflows/release.yml`): native clang builds darwin, and zig 0.14.1 builds `linux/*` and `windows/*` from the same host, so nothing has to be merged. Checked on 2026-09-17: zig on an arm64 Mac builds static musl Linux binaries and Windows PE binaries for amd64 and arm64. The same workflow builds a snapshot without publishing whenever `.goreleaser.yaml`, `install.sh` or the workflow changes on `main` or `staging`.
- macOS binaries codesigned + notarized; Windows binaries signed (when certificate is available).

### 22.3 Supply chain
- `checksums.txt` signed with **cosign** (keyless, GitHub OIDC); SBOM (syft) per release; SLSA provenance attestations.
- Reproducible builds (`-trimpath`, fixed `SOURCE_DATE_EPOCH`).
- Update notice: at most once per day, only on interactive TTY, never in CI, disabled via `MIGRAIL_NO_UPDATE_CHECK=1`. It's the **only** network call, and it sends no data beyond the HTTP request.

---

## 23. Testing & quality

### 23.1 Unit tests
- **Every rule:** `testdata/sql/MR101/{bad,good}/*.sql` with expected findings in `expected.json` (table-driven). Minimum 5 bad + 5 good cases, including version-boundary cases.
- Adapters: parsing fixtures per framework format (goose annotations, Drizzle breakpoints, Django `atomic`, …).
- Config: valid/invalid YAML fixtures with expected error frames.

### 23.2 Golden (snapshot) tests
- Terminal output for all screens in §19.5 at widths 60/80/120 × color profiles; stored in `testdata/golden/`; `make golden-update` regenerates.
- TUI tested with `teatest` (Bubble Tea test harness): scripted keypresses → final frame snapshots.
- JSON/SARIF outputs validated against schemas.

### 23.3 Lock verification harness (`tools/lockverify`), our credibility engine
For each lock-model row (§10.1) and each Postgres version 12–18 (testcontainers):
1. Create table with data (small + configurable large sizes).
2. Session A: `BEGIN; <statement>` (or run non-tx statements with a pg_sleep-instrumented approach / advisory barrier).
3. Session B: query `pg_locks` joined to `pg_class` → record actual lock modes.
4. Session C: attempt `SELECT`, `INSERT`, `UPDATE` with `lock_timeout=100ms` → record what's blocked.
5. Detect rewrite: compare `pg_class.relfilenode` before/after.
6. Output `lockmodel.verified.json`, and CI **fails if the lock model table disagrees** with observed behavior.

Runs nightly + on changes to `dialect/postgres/lockmodel`.

### 23.4 Framework integration tests
`testdata/projects/` contains tiny real apps (Django, Rails, Laravel, Prisma, EF Core, Knex, TypeORM, Sequelize, Alembic, Spring+Flyway, Go+goose). CI job per framework (Docker images with runtimes) runs `migrail check` and compares against expected findings, including capture mode.

### 23.5 Real-world corpus
`testdata/corpus/fetch.sh` pulls migrations from large OSS projects (e.g. GitLab, Discourse, Mastodon, Sentry, Zulip, Saleor, Supabase, Cal.com). Nightly job runs `--all` and tracks:
- findings per rule (trend dashboard in CI artifacts)
- **manually labeled sample** (true/false positive), target < 5% FP for errors before v1.0.

### 23.6 Fuzzing & robustness
- `go test -fuzz` on SQL splitter, annotation parsers, suppression comment parser, config loader.
- No panics on arbitrary input; parse errors always become MR901.

### 23.7 Benchmarks
`tools/bench` with synthetic repos (small/medium/huge); CI compares against `main` and fails on > 15% regression for budgets in §21.1.

### 23.8 Code quality gates
`golangci-lint` (strict preset), `go vet`, `govulncheck`, race detector in tests, coverage ≥ 80% for `rules/`, `analyze/`, `adapters/`.

---

## 24. Security, privacy, docs, community

### Security & privacy
- **No telemetry.** Ever, unless we add an explicit opt-in later, with a public schema of what's sent.
- Never writes to user databases; capture mode only uses containers it created (or DBs explicitly passed and verified empty).
- Secret redaction in all logs/outputs (URLs, `PASSWORD=`, tokens in captured command output).
- Framework CLI extraction executes user project code (e.g. Django settings). Documented clearly; `--no-exec` forces static-only mode.
- `SECURITY.md` with a private disclosure process.

### Documentation site (`<docs-domain>`, e.g. Astro Starlight or VitePress)
- Quickstart per framework (pick your stack → 3 steps).
- Rule reference, generated from rule metadata/markdown.
- "Zero-downtime migrations playbook" guides (expand/contract, backfills, constraints): SEO + real value.
- CI setup guides, configuration reference (generated from JSON schema), capture mode, live mode, FAQ.
- "migrail vs Atlas / squawk / strong_migrations" honest comparison page.

### Community
- `CONTRIBUTING.md` with "add a rule in 15 minutes" guide + rule scaffolder: `make new-rule ID=MR112 SLUG=…`.
- Issue templates: false positive (with SQL + version), new rule, new framework.
- Good-first-issue labels on best-practice rules and framework fix templates.

---

## 25. Open decisions

| # | Decision | Options | Recommendation |
|---|---|---|---|
| D1 | Product name | – | **Decided: `migrail`.** On 2026-09-17 it was free on npm, PyPI, RubyGems, NuGet, Packagist, crates, Homebrew, Docker Hub and as a GitHub name. Reserve package names before the first public release |
| D2 | License | Apache-2.0 / MIT | **Decided: Apache-2.0** (patent grant; enterprise friendly). `LICENSE` added in M0 |
| D3 | Rails primary strategy | capture vs static DSL | ship static DSL first (no Docker needed), capture as upgrade |
| D4 | Default DB version when unknown | oldest supported / latest | oldest supported (conservative) + notice |
| D5 | Min supported Postgres | 12 / 13 | 12 (still common in production), revisit yearly |
| D6 | Monetization (if any) | none / hosted dashboard / support | CLI + CI free forever; optional team dashboard later (§27) |
| D7 | Repository owner & docs domain | personal account / personal org · subdomain of own domain / dedicated domain | decide during branding; replace `<owner>` and `<docs-domain>` placeholders then |

---

## 26. Milestones & acceptance criteria

Built with **Claude Code** doing the implementation, so writing code is no longer the bottleneck. The time goes to things that can't be skipped:
- **Verification loops:** lock harness runs across PG 12–18, framework integration test containers, cross-platform builds.
- **Human review:** your review of design goldens, labeling false positives in the corpus, final product decisions.
- **External accounts & waits:** npm/PyPI/RubyGems/NuGet/Packagist accounts, Apple notarization, Homebrew tap, domain/DNS.

Durations are **focused working days** (a day = one solid Claude Code session with you reviewing). Total ≈ **4–5 weeks to v1.0**.

| Milestone | Days | Cumulative |
|---|---|---|
| M0 Foundations | 1 | day 1 |
| M1 Engine core | 2 | day 3 |
| M2 Beautiful CLI | 2 | day 5 |
| M3 v0.1 release | 3 | day 8 |
| M4 Every framework | 6 | day 14 |
| M5 App-aware | 5 | day 19 |
| M6 Trustworthy 1.0 | 5 | day 24 |

**How each milestone runs with Claude Code:** plan the milestone → implement in small verified slices (tests green before moving on) → run `/code-review` → you review goldens/UX → tag. Independent work (e.g. separate framework adapters, separate language extractors) can run in parallel worktrees.

### M0: Foundations (day 1)
- Repo scaffold (§7), Makefile/Taskfile, CI (lint, test, race), goreleaser skeleton building all targets with cgo.
- Competitive audit (§2) documented.
- Package names reserved on the main registries (D1). Owner and domain can wait (D7).
- **Done when:** `migrail version` builds and runs on macOS, Linux, Windows from CI artifacts.

### M1: Engine core (days 2–3)
- IR, Postgres dialect wrapper (pg_query), statement splitting with spans.
- Context tracker (tx state, new-objects set, timeouts).
- Rule registry + first 5 rules (MR101, MR104, MR107, MR201, MR302) with fixtures.
- JSON output.
- **Done when:** `migrail check file.sql -f json` produces correct findings for all fixtures.
- Implementation notes (2026-09-17):
  - The `Dialect` interface is defined where it's consumed (`internal/analyze`), not in `internal/dialect/dialect.go`, following CLAUDE.md. It grows as later consumers need more methods.
  - `check` defaults to `-f json` until M2 ships the pretty renderer, then the default switches to `pretty`.
  - Standalone `.sql` files passed to `check` are treated as non-transactional (how `psql -f` runs them). Adapters set the real transaction mode in M3.
  - MR201 can't see the current column type from migrations alone. When the new type allows a catalog-only change (`text`, `varchar`, `numeric`, `inet`, `timestamptz`, `citext`, `varbit`) and there's no computing `USING`, it reports a warning with `possible` confidence instead of an error.
  - `lockmodel` rows come from the PostgreSQL documentation. `make lockverify` arrives with the harness in M3 and must confirm them.

### M2: Beautiful CLI (days 4–5)
- Theme tokens, components (code frame, tree labels, summary bar, spinner), pretty & compact renderers.
- `explain`, `rules`, `completion`; golden tests.
- **Done when:** all §19.5 screens A–E, G–H and J match goldens at all widths/profiles.
- Implementation notes (2026-09-17):
  - glamour was dropped. Its chroma dependency adds about 6 ms of package init to every command, including `version`, which puts the §21.1 budget at risk. `explain` uses a small renderer for the fixed rule-doc format (headings, paragraphs, lists, tables, code blocks, inline code and links).
  - Background detection checks `COLORFGBG`, then asks the terminal through lipgloss only when stdin and stdout are both terminals. lipgloss waits up to 2 s instead of 100 ms, but it also sends a device-attributes query that nearly every terminal answers at once.
  - The summary omits the `◌ ignored` count and the header omits framework and git base until suppressions, adapters and git detection land in M3.
  - Fix code in findings is never truncated, so it can be copied. Code frames still truncate with `…`.
  - Spans that cover several lines use a severity-colored `┃` marker on each line instead of a caret underline.
  - Goldens live in `testdata/golden/` for screens A, B, C, D, E, G, H, J and help, at widths 60, 80 and 120 with truecolor, 16-color, no-color and ASCII profiles. `make golden-update` regenerates them.
  - Startup on an M-series Mac: `version` 5.3 ms, `check` 7.2 ms, `explain` 6.0 ms.

### M3: v0.1 release (days 6–8)
- Raw SQL adapters (golang-migrate, goose, Atlas dir, Flyway, Prisma, Drizzle, dbmate, sqitch).
- Git change detection, config, inline suppressions, all **v0.1** rules.
- Lock verification harness for v0.1 rules on PG 12–18.
- Homebrew tap + install script + GitHub release.
- **Done when:** runs on 3 real OSS repos with zero crashes; clean-run budget met; lock harness green.
- Implementation notes (2026-09-17):
  - MR304 ships off by default. Without code scanning it would report every `DROP TABLE` a second time next to MR502. It turns on by default with the code scanner in M5.
  - MR403 reports once per migration, at the first statement that locks an existing table without a `lock_timeout`.
  - MR106 covers table constraints (`ADD UNIQUE`, `ADD PRIMARY KEY`). Inline `ADD COLUMN … UNIQUE` isn't detected yet.
  - The config file is validated in Go (strict YAML decoding plus value checks) instead of with a JSON schema library. `schemas/config.schema.json` is still published for editors, and a test keeps its keys in sync with the config structs. v0.1 supports `version`, `db.dialect`, `db.version`, `projects`, `git`, `rules` severities, `policy.fail_on`, `policy.require_ignore_reason`, `ignore` and `output`. Rule options such as `lock_timeout_max`, `thresholds`, `codescan`, `capture` and `db.url_env` arrive with the features that use them.
  - Lock verification (2026-09-17): `tools/lockverify` runs every `lockmodel` entry and the lock conflict matrix against PostgreSQL 12, 13, 14, 15, 16, 17 and 18 in Docker, with 0 mismatches. It uses the Docker CLI and pgx instead of testcontainers-go. Scan claims aren't verified yet, and TRUNCATE and DROP TABLE skip the rewrite check because they replace or remove storage instead of rewriting rows.
  - Real repositories (2026-09-17, `check --all`): Mattermost (224 golang-migrate migrations), Supabase Auth (75 templated SQL migrations) and Cal.com (595 Prisma migrations) ran with zero crashes and zero rule errors. Supabase Auth led to masking Go template expressions before parsing.
  - Clean run with 3 changed goose migrations in a 203-migration repository: 40 ms on an M-series Mac, against the 150 ms budget.
  - Release (2026-09-17): a `v*` tag runs goreleaser on one macOS runner (§22.2). It creates a draft GitHub release with archives, `checksums.txt` signed by cosign keyless, a syft SBOM per archive and `install.sh`, and pushes a cask to `bbrainttech/homebrew-tap` using the `HOMEBREW_TAP_TOKEN` secret. The cask removes the quarantine attribute because macOS binaries aren't notarized yet. `install.sh` supports macOS and Linux and verifies the checksum; Windows users download the zip.
  - MR202 treats a small list of known volatile functions as definite, a list of known non-volatile functions as safe, and any other function in a default as a warning with `possible` confidence.

### M4: Every framework (days 9–14)
- Django, Alembic, Laravel, EF Core, Liquibase adapters (CLI); Rails static DSL; TypeORM static; Knex/Sequelize capture.
- Capture mode (§13) with caching.
- Framework-native fix templates for all v0.1 rules.
- `init`, `doctor`, screens F & I.
- SARIF/github/markdown/junit/gitlab formats; GitHub Action; pre-commit.
- npm, PyPI, RubyGems, Composer, NuGet, Docker, Scoop packages.
- **Done when:** every project in `testdata/projects/` passes integration tests; `npx migrail` / `pipx run migrail` / `gem`… work on clean machines. **Release v0.2.**
- Implementation notes (2026-09-18):
  - `-f sarif` writes SARIF 2.1.0 (`internal/report/sarif`), validated against the OASIS schema. Every rule is listed under `tool.driver.rules` with its docs as `help.markdown`. Results carry `partialFingerprints["migrail/v1"]`, `columnKind: unicodeCodePoints`, and URIs relative to the working directory under `%SRCROOT%`. Ignored findings are kept as results with `suppressions` (`inSource` for comments, `external` for config). Rule errors become `toolExecutionNotifications` and set `executionSuccessful: false`.
  - `-f github` (`internal/report/github`) prints one `::error`/`::warning`/`::notice` command per finding with `file`, `line`, `endLine`, and `col`/`endColumn` for single-line spans, titled `<ID> <title>`, with the why, fix and an `explain` hint as the message. Values are escaped per the workflow command rules. Ignored findings are skipped. It ends with a one-line count.
  - `-f markdown` (`internal/report/markdown`) writes a `## migrail` heading, a count line that names how many findings fail `--fail-on`, a summary table, and a `<details>` section per finding with the statement, lock, why, numbered fix steps and an `explain` hint. A clean run prints "No issues found". Ignored findings are left out and counted in the footer. Code fences grow when the SQL contains backticks, and table cells escape `|`, `&`, `<` and `>`.

### M5: App-aware (days 15–19)
- Codescan index + extractors for 7 languages; MR301/MR304/MR305 full logic incl. same-diff detection.
- Live DB mode + impact estimation + severity adjustments.
- Interactive TUI explorer.
- Baselines.
- **Done when:** codescan budget met on a 50k-file monorepo; TUI teatest suite green. **Release v0.3.**

### M6: Trustworthy 1.0 (days 20–24)
- Remaining v0.2 rules; corpus FP labeling; FP < 5% for errors.
- Docs site complete; comparison page; schemas frozen.
- Performance budgets all green; security review; signing/notarization.
- **Done when:** all acceptance criteria above + stable schemas. **Release v1.0** + launch (Show HN, Reddit r/golang r/django r/rails r/PostgreSQL, dev.to article "The 12 migrations that take production down").

### M7+: MySQL/MariaDB (v2), then SQL Server/SQLite/CockroachDB (v3)

---

## 27. Future (post-1.0)

- **More dialects** (§10.2–10.3).
- **Editor integration:** LSP server (`migrail lsp`) → diagnostics while writing migrations in VS Code, JetBrains, Neovim, Zed; VS Code extension with quick-fix code actions applying framework-native fixes.
- **Autofix:** `migrail fix` rewrites raw SQL migrations safely (e.g. add `CONCURRENTLY` + no-transaction annotation) with a diff preview.
- **Migration plan generator:** `migrail plan rename users.email email_address` → generates the full expand/contract migration series for your framework.
- **WASM rule plugins** for org-specific rules without forking.
- **Team dashboard (optional, self-hostable, built with Go + Gin + PostgreSQL + Redis):** migration history across repos, approval workflow for flagged migrations (e.g. "SRE must approve protected tables"), org-wide policy distribution, Slack notifications. Redis for job queues/caching of analysis results; Postgres as the system of record. The CLI never requires it.
