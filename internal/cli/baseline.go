package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/baseline"
	"github.com/bbrainttech/migrail/internal/discovery"
	"github.com/bbrainttech/migrail/internal/ui/components"
)

func newBaselineCommand(ui *uiFlags) *cobra.Command {
	opts := checkOptions{ui: ui, all: true, noBaseline: true, format: formatPretty, failOn: failOnNever}

	cmd := &cobra.Command{
		Use:   "baseline [path]...",
		Short: "Record today's findings so only new ones are reported",
		Long: "Check every migration and record the findings in " + baseline.FileName + ". After that, migrail check " +
			"hides those findings and reports only new ones. Commit the file and run this again to refresh it.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBaseline(cmd.Context(), cmd, args, opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&opts.dbVersion, "db-version", "", "PostgreSQL major version in production, such as 16 (default: 12)")
	flags.StringVar(&opts.framework, "framework", "", "migration tool to use instead of detecting it: "+strings.Join(discovery.AdapterNames(), ", "))
	flags.StringVarP(&opts.dir, "dir", "d", "", "project root to search for migrations (default: the repository root)")
	flags.StringVarP(&opts.baselinePath, "output", "o", "", "baseline `file` to write (default: "+baseline.FileName+" at the project root)")

	return cmd
}

func runBaseline(ctx context.Context, cmd *cobra.Command, args []string, opts checkOptions) error {
	dialect, dbVersion, _, err := prepareCheck(cmd, &opts)
	if err != nil {
		return err
	}

	opts.quiet = true

	report, err := analyzeProject(ctx, cmd, args, opts, dialect, dbVersion)
	if err != nil {
		return err
	}

	file := baseline.New(report.result.Findings)

	path := opts.baselinePath
	if path == "" {
		path = filepath.Join(opts.root, baseline.FileName)
	}

	if err := file.Save(path); err != nil {
		return err
	}

	_, err = fmt.Fprintf(
		cmd.OutOrStdout(),
		"Recorded %d %s from %d %s in %s\nmigrail check now reports only new findings. Commit this file to share it with your team.\n",
		len(file.Findings), components.Plural(len(file.Findings), "finding", "findings"),
		len(report.migrations), components.Plural(len(report.migrations), "migration", "migrations"),
		filepath.ToSlash(path),
	)
	if err != nil {
		return internalError(fmt.Errorf("write baseline summary: %w", err))
	}

	return nil
}
