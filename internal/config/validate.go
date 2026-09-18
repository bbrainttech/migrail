package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Catalog struct {
	RuleIDs    []string
	RuleSlugs  []string
	Frameworks []string
}

var (
	severities   = []string{"error", "warning", "notice", "off"}
	failOnLevels = []string{"error", "warning", "notice", "never"}
	checkModes   = []string{"changed", "all"}
	formats      = []string{"pretty", "json", "sarif"}
	themes       = []string{"auto", "dark", "light", "mono"}
	switches     = []string{"auto", "always", "never"}
	dialects     = []string{"postgres"}
)

func validate(cfg Config, path string, data []byte) error {
	if cfg.Version != CurrentVersion {
		return errorAt(path, data, "$.version", fmt.Sprintf("version must be %d", CurrentVersion), nil)
	}

	checks := []struct {
		yamlPath string
		value    string
		label    string
		options  []string
	}{
		{yamlPath: "$.db.dialect", value: cfg.DB.Dialect, label: "db.dialect", options: dialects},
		{yamlPath: "$.git.check", value: cfg.Git.Check, label: "git.check", options: checkModes},
		{yamlPath: "$.policy.fail_on", value: cfg.Policy.FailOn, label: "policy.fail_on", options: failOnLevels},
		{yamlPath: "$.output.format", value: cfg.Output.Format, label: "output.format", options: formats},
		{yamlPath: "$.output.theme", value: cfg.Output.Theme, label: "output.theme", options: themes},
		{yamlPath: "$.output.color", value: cfg.Output.Color, label: "output.color", options: switches},
		{yamlPath: "$.output.hyperlinks", value: cfg.Output.Hyperlinks, label: "output.hyperlinks", options: switches},
	}

	for _, check := range checks {
		if !contains(check.options, check.value) {
			return errorAt(path, data, check.yamlPath, fmt.Sprintf("invalid %s %q", check.label, check.value), check.options)
		}
	}

	if cfg.DB.Version != "" {
		major, err := strconv.Atoi(strings.SplitN(cfg.DB.Version, ".", 2)[0])
		if err != nil || major < 12 || major > 18 {
			return errorAt(path, data, "$.db.version", fmt.Sprintf("invalid db.version %q: migrail supports PostgreSQL 12 to 18", cfg.DB.Version), nil)
		}
	}

	return nil
}

func (c Config) ValidateCatalog(catalog Catalog) error {
	data := []byte("")

	if c.Path != "" {
		if loaded, err := readSource(c.Path); err == nil {
			data = loaded
		}
	}

	for key, setting := range c.Rules {
		if !contains(catalog.RuleIDs, key) && !contains(catalog.RuleSlugs, key) {
			message := fmt.Sprintf("unknown rule %q", key)

			result := errorAt(c.Path, data, "$.rules."+key, message, nil)
			result.Suggestion = closest(key, append(append([]string{}, catalog.RuleIDs...), catalog.RuleSlugs...))

			return result
		}

		if !contains(severities, setting.Severity) {
			return errorAt(c.Path, data, "$.rules."+key, fmt.Sprintf("invalid severity %q for %s", setting.Severity, key), severities)
		}
	}

	for i, project := range c.Projects {
		if !contains(catalog.Frameworks, project.Framework) {
			return errorAt(c.Path, data, fmt.Sprintf("$.projects[%d].framework", i), fmt.Sprintf("unknown framework %q", project.Framework), catalog.Frameworks)
		}

		if project.Path == "" {
			return errorAt(c.Path, data, fmt.Sprintf("$.projects[%d]", i), "project needs a path", nil)
		}
	}

	return nil
}
