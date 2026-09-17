package cli

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/analyze"
	"github.com/bbrainttech/migrail/internal/config"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/discovery"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/report/jsonreport"
	"github.com/bbrainttech/migrail/internal/report/pretty"
	"github.com/bbrainttech/migrail/internal/rules"
	"github.com/bbrainttech/migrail/internal/suppress"
	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/progress"
	"github.com/bbrainttech/migrail/internal/ui/term"
)

const (
	exitFindings  = 1
	stdinArgument = "-"
	stdinPath     = "<stdin>"
	formatJSON    = "json"
	formatPretty  = "pretty"
	failOnNever   = "never"
)

var formats = []string{formatPretty, formatJSON}

var failOnLevels = []string{string(ir.SeverityError), string(ir.SeverityWarning), string(ir.SeverityNotice), failOnNever}

type checkOptions struct {
	format    string
	dbVersion string
	failOn    string
	rules     []string
	skipRules []string
	framework string
	dir       string
	base      string
	all       bool
	root      string
	cfg       config.Config
	compact   bool
	quiet     bool
	verbose   bool
	ci        bool
	ui        *uiFlags
}

func newCheckCommand(ui *uiFlags) *cobra.Command {
	opts := checkOptions{ui: ui}

	cmd := &cobra.Command{
		Use:   "check [path]... | -",
		Short: "Check SQL migrations for locks, rewrites and breaking changes",
		Long: "Check migrations for statements that lock tables, rewrite them or break running code.\n\n" +
			"With no arguments, migrail finds the migrations in the repository and detects the migration tool. " +
			"Pass migration files or directories to check only those, or - to read SQL from standard input.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCheck(cmd.Context(), cmd, args, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.format, "format", "f", formatPretty, "output format: pretty or json")
	flags.StringVar(&opts.dbVersion, "db-version", "", "PostgreSQL major version in production, such as 16 (default: 12)")
	flags.StringVar(&opts.failOn, "fail-on", string(ir.SeverityError), "exit with code 1 on findings at or above: error, warning, notice or never")
	flags.StringSliceVarP(&opts.rules, "rule", "r", nil, "only run these rules, by ID or slug")
	flags.StringSliceVar(&opts.skipRules, "skip-rule", nil, "skip these rules, by ID or slug")
	flags.StringVar(&opts.framework, "framework", "", "migration tool to use instead of detecting it: "+strings.Join(discovery.AdapterNames(), ", "))
	flags.StringVarP(&opts.dir, "dir", "d", "", "project root to search for migrations (default: the repository root)")
	flags.BoolVar(&opts.all, "all", false, "check every migration, not only the ones changed since the base branch")
	flags.StringVar(&opts.base, "base", "", "git branch or commit to compare against (default: origin/HEAD, main or master)")
	flags.BoolVar(&opts.compact, "compact", false, "print one line per finding")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "print only findings and the summary")
	flags.BoolVarP(&opts.verbose, "verbose", "v", false, "print each phase with its timing")
	flags.BoolVar(&opts.ci, "ci", false, "CI mode: no color, links or spinners unless forced")

	return cmd
}

func runCheck(ctx context.Context, cmd *cobra.Command, args []string, opts checkOptions) error {
	started := time.Now()
	dialect := pg.New()

	if err := applyConfig(cmd, &opts); err != nil {
		return err
	}

	if err := opts.ui.validate(); err != nil {
		return err
	}

	dbVersion, err := validateCheckOptions(dialect, opts)
	if err != nil {
		return err
	}

	settings := opts.ui.settings(ciEnabled(opts.ci))
	stderr := newDisplay(cmd.ErrOrStderr(), settings)
	phases := progress.New(stderr.out, cmd.ErrOrStderr(), stderr.theme, progress.Options{
		Animate: stderr.caps.TTY && !settings.CI && !opts.quiet,
		Verbose: opts.verbose,
	})

	finish := phases.Start("Discovering", fmt.Sprintf("%d %s", len(args), components.Plural(len(args), "file", "files")))

	migrations, scope, err := collectMigrations(ctx, cmd, dialect, args, opts, stderr)
	if err != nil {
		finish.Abort()

		return err
	}

	finish.Done("Discovered", fmt.Sprintf("%d %s", len(migrations), components.Plural(len(migrations), "migration", "migrations")))

	if opts.dbVersion == "" && !opts.quiet {
		writeNotice(stderr, fmt.Sprintf("assuming PostgreSQL %s. Set --db-version for advice that matches your version.", dbVersion))
	}

	finish = phases.Start("Analyzing", fmt.Sprintf("%d rules", len(rules.All())))

	result, err := analyze.Run(ctx, dialect, migrations, rules.All(), analyze.Options{
		DBVersion: dbVersion,
		Only:      opts.rules,
		Skip:      opts.skipRules,
	})
	if err != nil {
		finish.Abort()

		if errors.Is(err, context.Canceled) {
			return err
		}

		return internalError(err)
	}

	result = applySuppressions(result, migrations, dialect, opts)

	finish.Done("Analyzed", fmt.Sprintf("%d %s %s %d rules", result.Statements, components.Plural(result.Statements, "statement", "statements"), stderr.theme.Symbols.Dot, result.RulesRun))

	if err := writeCheckReport(cmd, opts, settings, dialect, dbVersion, migrations, scope, result, time.Since(started)); err != nil {
		return internalError(err)
	}

	return checkOutcome(result, opts.failOn, opts.format == formatPretty)
}

func collectMigrations(
	ctx context.Context,
	cmd *cobra.Command,
	dialect *pg.Dialect,
	args []string,
	opts checkOptions,
	stderr display,
) ([]*ir.Migration, changeScope, error) {
	migrations, err := loadMigrations(ctx, dialect, args, opts, cmd.InOrStdin())
	if err != nil {
		return nil, changeScope{}, err
	}

	scope := changeScope{}
	explicitPaths := len(args) > 0

	if !explicitPaths || opts.all {
		notice := func(message string) {
			if !opts.quiet {
				writeNotice(stderr, message)
			}
		}

		migrations, scope, err = applyGitScope(ctx, migrations, explicitPaths, opts, notice)
		if err != nil {
			return nil, changeScope{}, err
		}
	}

	for _, migration := range migrations {
		if err := discovery.ParseMigration(migration, dialect.Parse); err != nil {
			return nil, changeScope{}, internalError(err)
		}
	}

	return migrations, scope, nil
}

func writeCheckReport(
	cmd *cobra.Command,
	opts checkOptions,
	settings term.Settings,
	dialect *pg.Dialect,
	dbVersion ir.Version,
	migrations []*ir.Migration,
	scope changeScope,
	result analyze.Result,
	elapsed time.Duration,
) error {
	if opts.format == formatJSON {
		return jsonreport.Write(cmd.OutOrStdout(), jsonreport.Input{
			ToolVersion: currentBuildInfo().Version,
			Dialect:     dialect.Name(),
			DBVersion:   dbVersion,
			Migrations:  migrations,
			Result:      result,
			Base:        scope.Base,
			ChangedOnly: scope.ChangedOnly,
		})
	}

	stdout := newDisplay(cmd.OutOrStdout(), settings)
	options := pretty.Options{
		Theme:   stdout.theme,
		Width:   stdout.caps.ContentWidth(),
		Compact: opts.compact || opts.quiet || stdout.caps.Compact(),
	}

	if stdout.caps.Hyperlinks {
		options.Link = fileLink
	}

	return pretty.Write(stdout.out, pretty.Input{
		ToolVersion: currentBuildInfo().Version,
		Dialect:     dialect.Name(),
		DBVersion:   dbVersion,
		Migrations:  migrations,
		Result:      result,
		FailOn:      opts.failOn,
		Elapsed:     elapsed,
		Highlight:   dialect.Highlight,
		Base:        scope.Base,
		ChangedOnly: scope.ChangedOnly,
	}, options)
}

func applySuppressions(result analyze.Result, migrations []*ir.Migration, dialect *pg.Dialect, opts checkOptions) analyze.Result {
	all := rules.All()
	active := map[string]bool{}

	for _, rule := range analyze.ActiveRules(all, dialect.Name(), analyze.Options{Only: opts.rules, Skip: opts.skipRules}) {
		active[rule.Meta().ID] = true
	}

	ignores := make([]suppress.ConfigIgnore, 0, len(opts.cfg.Ignore))
	for _, ignore := range opts.cfg.Ignore {
		ignores = append(ignores, suppress.ConfigIgnore{Path: ignore.Path, Rule: ignore.Rule, Reason: ignore.Reason})
	}

	result = suppress.Apply(result, migrations, suppress.Options{
		RequireReason: opts.cfg.RequireIgnoreReason(),
		Ignores:       ignores,
		Rules:         all,
		Dialect:       dialect,
		PathOf:        relativeToRoot(opts.root),
	})
	severityOverrides(opts.cfg, result.Findings)

	kept := result.Findings[:0]
	for _, finding := range result.Findings {
		if active[finding.RuleID] {
			kept = append(kept, finding)
		}
	}

	result.Findings = kept

	return result
}

func writeNotice(d display, message string) {
	t := d.theme
	_, _ = fmt.Fprintln(d.out, t.Notice.Render(t.Symbols.Notice)+" "+t.Strong(t.Notice).Render("notice")+" "+t.Fg.Render(message))
}

func validateCheckOptions(dialect *pg.Dialect, opts checkOptions) (ir.Version, error) {
	if _, ok := discovery.AdapterNamed(opts.framework); opts.framework != "" && !ok {
		return ir.Version{}, fmt.Errorf("unknown --framework %q: use %s", opts.framework, strings.Join(discovery.AdapterNames(), ", "))
	}

	if !slices.Contains(formats, opts.format) {
		return ir.Version{}, fmt.Errorf("unsupported format %q: use %s", opts.format, strings.Join(formats, " or "))
	}

	if !slices.Contains(failOnLevels, opts.failOn) {
		return ir.Version{}, fmt.Errorf("invalid --fail-on %q: use %s", opts.failOn, strings.Join(failOnLevels, ", "))
	}

	if opts.dbVersion == "" {
		return dialect.DefaultVersion(), nil
	}

	version, err := ir.ParseVersion(opts.dbVersion)
	if err != nil {
		return ir.Version{}, fmt.Errorf("invalid --db-version: %w", err)
	}

	lowest, highest := dialect.SupportedVersions()
	if version.Major < lowest.Major || version.Major > highest.Major {
		return ir.Version{}, fmt.Errorf("unsupported --db-version %s: migrail supports PostgreSQL %s to %s", version, lowest, highest)
	}

	return version, nil
}

func checkOutcome(result analyze.Result, failOn string, summarized bool) error {
	if len(result.RuleErrors) > 0 {
		return internalError(fmt.Errorf("%d rule checks failed, so findings may be incomplete: %s", len(result.RuleErrors), result.RuleErrors[0].Message))
	}

	if failOn == failOnNever {
		return nil
	}

	failing := 0

	for _, finding := range result.Findings {
		if finding.Suppressed == nil && finding.Severity.AtLeast(ir.Severity(failOn)) {
			failing++
		}
	}

	if failing == 0 {
		return nil
	}

	noun := "findings"
	if failing == 1 {
		noun = "finding"
	}

	return &exitError{code: exitFindings, err: fmt.Errorf("%d %s at or above %q", failing, noun, failOn), silent: summarized}
}
