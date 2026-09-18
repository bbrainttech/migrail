# CLAUDE.md

migrail is a free, open source CLI that catches dangerous database migrations before they reach production. It reads migrations the way the database will execute them, reports locks, table rewrites and app-breaking changes, and suggests fixes in the user's own framework syntax.

`PLAN.md` is the source of truth for scope, architecture, rules, terminal design and milestones. Read the relevant section before starting any task. If an implementation decision conflicts with the plan, stop and ask; don't silently diverge. When a decision changes the plan, update `PLAN.md` in the same change.

## Stack

- Go (≥ 1.26.4), single binary, cgo enabled
- CLI: cobra · Terminal UI: lipgloss, colorprofile, bubbletea, bubbles, huh (markdown for `explain` is rendered in `internal/ui/markdown`, not glamour, to protect the startup budget)
- Postgres parser: pganalyze/pg_query_go · Code parsing: tree-sitter
- Live DB: pgx/v5 · Config: goccy/go-yaml + JSON schema
- Tests: testcontainers-go, teatest, golden files · Release: goreleaser

Add a dependency only when the plan calls for it or the standard library can't do the job reasonably. Load the `golang-popular-libraries` and `golang-dependency-management` skills before adding one.

## Layout

```
cmd/migrail/          entrypoint only
internal/cli/         one file per command
internal/config/      load, merge, validate
internal/discovery/   project + framework detection, git change detection
internal/adapters/    framework → SQL (one package per framework, capture/)
internal/dialect/     dialect interface, postgres/ (parser, lock model)
internal/ir/          dialect-neutral statement model
internal/analyze/     context tracker, rule runner
internal/rules/       postgres/, common/ (one file per rule + .md doc + test)
internal/fixes/       framework-native fix templates
internal/codescan/    symbol index + per-language extractors
internal/livedb/      read-only stats, impact estimates
internal/suppress/    inline ignores, baselines
internal/report/      pretty, tui, json, sarif, github, markdown, junit
internal/ui/          theme tokens, components, terminal detection
pkg/migrail/          small public API
testdata/             sql fixtures, framework projects, golden output
tools/                lockverify, gendocs, bench
packaging/            npm, pypi, rubygems, composer, homebrew, scoop, docker
```

## Commands

```
make build            build ./bin/migrail
make test             unit tests with -race
make lint             golangci-lint
make golden-update    regenerate terminal snapshots (review the diff)
make lockverify       run lock verification against Postgres 12–18
make bench            benchmarks compared against budgets
```

Run `make lint test` before calling any task done. Keep these targets current as the Makefile evolves.

## Code style

- **No comments.** Code must explain itself through naming, small functions and clear types. Don't write doc comments, inline comments, TODOs or commented-out code. The only exceptions are directives the toolchain needs to work (`//go:build`, `//go:embed`, `//go:generate`, cgo preambles, `//nolint:<linter>` when unavoidable).
- Clean, small, focused packages. No `utils`, `helpers` or `common` grab-bags (`rules/common` is a domain name: dialect-agnostic rules).
- Accept interfaces, return structs. Define interfaces where they're consumed.
- Wrap errors with context (`fmt.Errorf("extract django migrations: %w", err)`), handle each error once, no panics outside `main` except recovered rule panics.
- Pass `context.Context` through every call that does I/O, spawns processes or can be cancelled.
- Table-driven tests. Every rule has bad and good fixtures under `testdata/sql/<ID>/`.
- Follow the installed Go skills in `.claude/skills/` (code-style, naming, error-handling, concurrency, context, safety, testing, performance, benchmark, lint, security, project-layout, structs-interfaces, database, documentation, troubleshooting, modernize).

## User-facing content

Use the **unslop** skill for all user-facing text before it ships:
- CLI output: finding titles, "why" and "fix" text, errors, notices, prompts, help text
- Rule docs (`internal/rules/**/*.md`) and `explain` output
- README, docs site, CHANGELOG, release notes
- GitHub Action PR comments and job summaries, markdown/SARIF help text

Voice: plain, precise and calm. Say what happens, where, why it's dangerous and exactly what to do. No hype, no filler, no exclamation marks, no emoji in output.

## Non-negotiables

- **Correct before clever.** Never report "safe" when unsure. Surface uncertainty as a notice.
- **Read-only.** Never write to a user's database. Capture mode only touches containers or empty databases migrail created or verified.
- **Rule IDs are permanent.** Never renumber or reuse an ID. Rule metadata and docs live next to the rule and generate `explain`, `rules`, SARIF and the docs site.
- **Lock claims are verified.** Any change to `internal/dialect/postgres/lockmodel` must pass `make lockverify`.
- **Streams:** findings and reports go to stdout, progress and notices to stderr.
- **Terminal design follows PLAN.md §19 exactly:** tokens, symbols, layouts, width rules, `NO_COLOR`, ASCII fallback. Visual changes update golden files in the same change.
- **Performance budgets (PLAN.md §21) are requirements.** Don't add work to the startup path; lazy-load heavy packages.
- **No telemetry.** The only network call is the opt-out update check, never in CI.
- **Secrets never appear** in output, logs or debug traces. Redact database URLs everywhere.

## Environment variables

Each time you add an env var:
1. Add the key to `.env.example` first, with a very brief comment above it saying what it's for. Use an empty or safe placeholder value, never a real secret.
2. Then add it to `.env.local` with the real value.

`.env.example` is committed. `.env.local` and every other `.env*` file are gitignored.

```
# Read-only Postgres URL used by live database integration tests
MIGRAIL_TEST_DATABASE_URL=
```

## README

`README.md` is the public face of the project. Write it for people who find the repository: what migrail is, why they'd use it, how to install and run it, and what's coming. Keep internal notes, plan details and working status out of it; those belong in `PLAN.md`.

Update the README in the same change whenever a user-visible feature ships or changes: move it from planned to available, add real install and usage commands, refresh examples and output samples, and tick the roadmap. Edit the existing sections in place rather than appending notes. Never document something that doesn't work yet as if it does; label planned features as planned.

## Keeping docs in sync

Every change that adds a feature or changes behavior updates the docs it affects **in the same commit**. Code and docs never ship out of step. Before committing, check each place below and update the ones the change touches:

| Change | Update |
|---|---|
| New or changed command, flag, format, config key or install step | `README.md` usage/flag tables and examples, command help text, help goldens |
| New or changed rule, or a rule's behavior, message or fix | the rule's `.md` doc next to it, `testdata/sql/<ID>/` fixtures, README rule table |
| Config keys or allowed values | `internal/config/schema.json` and `schemas/config.schema.json` (keep them identical), config validation, README config example |
| Output formats or report fields | the reporter's goldens, README output section, `schemas/` when a schema exists |
| Adapter or transaction-mode behavior | README "Supported stacks" and conventions text, `PLAN.md` §12.3 table |
| Scope, design or plan decision | `PLAN.md` (the relevant section and the milestone's implementation notes) |
| Install, uninstall or release process | README Installation, `install.sh --help`, `PLAN.md` §22 |
| Milestone "Done when" met | README roadmap checkbox |

If a change needs no doc update, that's fine, but decide it deliberately rather than by skipping the check. Mention in the final summary which docs were updated.

## Workflow

1. Read the milestone and sections involved in `PLAN.md`.
2. Implement in small vertical slices. Each slice builds, passes tests and lint.
3. For terminal output, run the binary and look at the result at 60, 80 and 120 columns before updating goldens.
4. Run `/code-review` at the end of a milestone.
5. Commit and push after finishing each fix, feature or user request, once `make lint test` passes and the affected docs are updated (see "Keeping docs in sync"). Use small focused commits with a `scope: summary` message, and push to the current branch. Never commit secrets or `.env*` files other than `.env.example`.
6. When a milestone's "Done when" criteria are met, tick its box in the README roadmap in the same commit.
