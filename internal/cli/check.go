package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/report/jsonreport"
	"github.com/bbrainttech/migrail/internal/report/pretty"
	"github.com/bbrainttech/migrail/internal/rules"
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
	compact   bool
	quiet     bool
	verbose   bool
	ci        bool
	ui        *uiFlags
}

func newCheckCommand(ui *uiFlags) *cobra.Command {
	opts := checkOptions{ui: ui}

	cmd := &cobra.Command{
		Use:   "check <file.sql>... | -",
		Short: "Check SQL migrations for locks, rewrites and breaking changes",
		Long: "Check SQL migration files for statements that lock tables, rewrite them or break running code.\n\n" +
			"Pass one or more .sql files, or - to read SQL from standard input.",
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
	flags.BoolVar(&opts.compact, "compact", false, "print one line per finding")
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "print only findings and the summary")
	flags.BoolVarP(&opts.verbose, "verbose", "v", false, "print each phase with its timing")
	flags.BoolVar(&opts.ci, "ci", false, "CI mode: no color, links or spinners unless forced")

	return cmd
}

func runCheck(ctx context.Context, cmd *cobra.Command, args []string, opts checkOptions) error {
	started := time.Now()
	dialect := pg.New()

	dbVersion, err := validateCheckOptions(dialect, args, opts)
	if err != nil {
		return err
	}

	settings := opts.ui.settings(ciEnabled(opts.ci))
	stderr := newDisplay(cmd.ErrOrStderr(), settings)
	phases := progress.New(stderr.out, stderr.theme, progress.Options{
		Animate: stderr.caps.TTY && !settings.CI && !opts.quiet,
		Verbose: opts.verbose,
	})

	finish := phases.Start("Discovering", fmt.Sprintf("%d %s", len(args), components.Plural(len(args), "file", "files")))

	migrations, err := loadMigrations(dialect, args, cmd.InOrStdin())
	if err != nil {
		finish.Abort()

		return err
	}

	finish.Done("Discovered", fmt.Sprintf("%d %s %s sql", len(migrations), components.Plural(len(migrations), "migration", "migrations"), stderr.theme.Symbols.Dot))

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

	finish.Done("Analyzed", fmt.Sprintf("%d %s %s %d rules", result.Statements, components.Plural(result.Statements, "statement", "statements"), stderr.theme.Symbols.Dot, result.RulesRun))

	if err := writeCheckReport(cmd, opts, settings, dialect, dbVersion, migrations, result, time.Since(started)); err != nil {
		return internalError(err)
	}

	return checkOutcome(result, opts.failOn, opts.format == formatPretty)
}

func writeCheckReport(
	cmd *cobra.Command,
	opts checkOptions,
	settings term.Settings,
	dialect *pg.Dialect,
	dbVersion ir.Version,
	migrations []*ir.Migration,
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
	}, options)
}

func writeNotice(d display, message string) {
	t := d.theme
	_, _ = fmt.Fprintln(d.out, t.Notice.Render(t.Symbols.Notice)+" "+t.Strong(t.Notice).Render("notice")+" "+t.Fg.Render(message))
}

func validateCheckOptions(dialect *pg.Dialect, args []string, opts checkOptions) (ir.Version, error) {
	if len(args) == 0 {
		return ir.Version{}, errors.New("no migrations to check: pass SQL files, or - to read standard input")
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

func loadMigrations(dialect *pg.Dialect, args []string, stdin io.Reader) ([]*ir.Migration, error) {
	if slices.Contains(args, stdinArgument) && len(args) > 1 {
		return nil, errors.New("- reads standard input and can't be combined with file paths")
	}

	migrations := make([]*ir.Migration, 0, len(args))

	for _, arg := range args {
		path, source, err := readMigration(arg, stdin)
		if err != nil {
			return nil, err
		}

		statements, err := dialect.Parse(source)
		if err != nil {
			return nil, internalError(fmt.Errorf("parse %s: %w", path, err))
		}

		migrations = append(migrations, &ir.Migration{
			ID:          "sql:" + path,
			Name:        strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
			Framework:   "sql",
			SourcePath:  path,
			Source:      source,
			Direction:   ir.DirectionUp,
			TxMode:      ir.TxModeNonTransactional,
			Statements:  statements,
			ChangeState: ir.ChangeStateNew,
			Origin:      ir.OriginRawSQL,
		})
	}

	return migrations, nil
}

func readMigration(arg string, stdin io.Reader) (path, source string, err error) {
	if arg == stdinArgument {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", "", internalError(fmt.Errorf("read standard input: %w", err))
		}

		return stdinPath, string(data), nil
	}

	path = filepath.ToSlash(filepath.Clean(arg))

	info, err := os.Stat(filepath.FromSlash(path))
	if err != nil {
		return "", "", fmt.Errorf("read migration %s: %w", path, err)
	}

	if info.IsDir() {
		return "", "", fmt.Errorf("%s is a directory: pass the SQL files to check", path)
	}

	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		return "", "", fmt.Errorf("read migration %s: %w", path, err)
	}

	return path, string(data), nil
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
		if finding.Severity.AtLeast(ir.Severity(failOn)) {
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
