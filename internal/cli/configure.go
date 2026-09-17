package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/config"
	"github.com/bbrainttech/migrail/internal/discovery"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/rules"
)

func applyConfig(cmd *cobra.Command, opts *checkOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return internalError(fmt.Errorf("find working directory: %w", err))
	}

	opts.root = opts.dir
	if opts.root == "" {
		opts.root = discovery.FindRoot(cwd)
	}

	path, found := config.Find(opts.root)
	if !found {
		return nil
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	if err := cfg.ValidateCatalog(ruleCatalog()); err != nil {
		return err
	}

	opts.cfg = cfg
	overrideOptions(cmd, opts, cfg)

	return nil
}

func overrideOptions(cmd *cobra.Command, opts *checkOptions, cfg config.Config) {
	flags := cmd.Flags()

	setIfUnchanged(flags.Changed("db-version"), &opts.dbVersion, cfg.DB.Version)
	setIfUnchanged(flags.Changed("base"), &opts.base, cfg.Git.Base)
	setIfUnchanged(flags.Changed("fail-on"), &opts.failOn, cfg.Policy.FailOn)
	setIfUnchanged(flags.Changed("format"), &opts.format, cfg.Output.Format)
	setIfUnchanged(flags.Changed("theme"), &opts.ui.theme, cfg.Output.Theme)
	setIfUnchanged(flags.Changed("color"), &opts.ui.color, cfg.Output.Color)

	opts.all = opts.all || !flags.Changed("all") && cfg.Git.Check == "all"
	opts.compact = opts.compact || !flags.Changed("compact") && cfg.Output.Compact
	opts.ui.noHyperlinks = opts.ui.noHyperlinks || !flags.Changed("no-hyperlinks") && cfg.Output.Hyperlinks == "never"

	if flags.Changed("rule") {
		return
	}

	for key, setting := range cfg.Rules {
		if setting.Severity == string(ir.SeverityOff) {
			opts.skipRules = append(opts.skipRules, key)
		}
	}
}

func setIfUnchanged(changed bool, target *string, value string) {
	if !changed && value != "" {
		*target = value
	}
}

func ruleCatalog() config.Catalog {
	catalog := config.Catalog{Frameworks: discovery.AdapterNames()}

	for _, rule := range rules.All() {
		catalog.RuleIDs = append(catalog.RuleIDs, rule.Meta().ID)
		catalog.RuleSlugs = append(catalog.RuleSlugs, rule.Meta().Slug)
	}

	return catalog
}

func severityOverrides(cfg config.Config, findings []ir.Finding) {
	if len(cfg.Rules) == 0 {
		return
	}

	for i := range findings {
		for _, key := range []string{findings[i].RuleID, findings[i].Slug} {
			setting, ok := cfg.Rules[key]
			if ok && setting.Severity != "" && setting.Severity != string(ir.SeverityOff) {
				findings[i].Severity = ir.Severity(setting.Severity)
			}
		}
	}
}

func relativeToRoot(root string) func(*ir.Migration) string {
	cwd, err := os.Getwd()

	return func(migration *ir.Migration) string {
		if err != nil {
			return migration.SourcePath
		}

		absolute := filepath.Join(cwd, filepath.FromSlash(migration.SourcePath))

		relative, relErr := filepath.Rel(absolutePath(cwd, root), absolute)
		if relErr != nil {
			return migration.SourcePath
		}

		return filepath.ToSlash(relative)
	}
}
