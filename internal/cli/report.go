package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bbrainttech/migrail/internal/analyze"
	pg "github.com/bbrainttech/migrail/internal/dialect/postgres"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/report/github"
	"github.com/bbrainttech/migrail/internal/report/jsonreport"
	"github.com/bbrainttech/migrail/internal/report/junit"
	"github.com/bbrainttech/migrail/internal/report/markdown"
	"github.com/bbrainttech/migrail/internal/report/pretty"
	"github.com/bbrainttech/migrail/internal/report/sarif"
	"github.com/bbrainttech/migrail/internal/ui/term"
)

const (
	outputFileMode   = 0o644
	outputDirMode    = 0o755
	githubActionsEnv = "GITHUB_ACTIONS"
	githubSummaryEnv = "GITHUB_STEP_SUMMARY"
)

var formatsByExtension = map[string]string{
	".sarif":    formatSARIF,
	".json":     formatJSON,
	".md":       formatMarkdown,
	".markdown": formatMarkdown,
	".xml":      formatJUnit,
	".txt":      formatPretty,
}

type checkReport struct {
	dialect    *pg.Dialect
	dbVersion  ir.Version
	migrations []*ir.Migration
	scope      changeScope
	result     analyze.Result
	failOn     string
	elapsed    time.Duration
}

type outputTarget struct {
	format string
	path   string
}

func parseOutputs(specs []string) ([]outputTarget, error) {
	targets := make([]outputTarget, 0, len(specs))

	for _, spec := range specs {
		target, err := parseOutput(spec)
		if err != nil {
			return nil, err
		}

		targets = append(targets, target)
	}

	return targets, nil
}

func parseOutput(spec string) (outputTarget, error) {
	if format, path, ok := strings.Cut(spec, "="); ok && slices.Contains(formats, format) {
		if path == "" {
			return outputTarget{}, fmt.Errorf("invalid --output %q: add a file path after %s=", spec, format)
		}

		return outputTarget{format: format, path: path}, nil
	}

	if spec == "" {
		return outputTarget{}, errors.New("invalid --output: pass a file path, such as migrail.sarif")
	}

	format, ok := formatsByExtension[strings.ToLower(filepath.Ext(spec))]
	if !ok {
		return outputTarget{}, fmt.Errorf(
			"can't tell the format of --output %q from its extension: name the format, such as sarif=%s. Formats: %s",
			spec, spec, strings.Join(formats, ", "),
		)
	}

	return outputTarget{format: format, path: spec}, nil
}

func writeOutputs(report checkReport, targets []outputTarget) error {
	fileSettings := term.Settings{Color: term.ColorNever, NoHyperlinks: true, CI: true}

	for _, target := range targets {
		var buffer bytes.Buffer

		if err := renderReport(&buffer, target.format, report, fileSettings, false); err != nil {
			return internalError(err)
		}

		if err := os.MkdirAll(filepath.Dir(target.path), outputDirMode); err != nil {
			return fmt.Errorf("create directory for --output %s: %w", target.path, err)
		}

		if err := os.WriteFile(target.path, buffer.Bytes(), outputFileMode); err != nil {
			return fmt.Errorf("write --output %s: %w", target.path, err)
		}
	}

	return nil
}

func writeStdout(w io.Writer, opts checkOptions, settings term.Settings, report checkReport) error {
	if err := renderReport(w, opts.format, report, settings, opts.compact || opts.quiet); err != nil {
		return err
	}

	if opts.format == formatPretty && inGitHubActions(opts.ui.env) {
		return github.Write(w, github.Input{Result: report.result, SkipCount: true})
	}

	return nil
}

func writeGitHubSummary(report checkReport, env term.Env) error {
	path := env[githubSummaryEnv]
	if !inGitHubActions(env) || path == "" {
		return nil
	}

	var buffer bytes.Buffer
	if err := renderReport(&buffer, formatMarkdown, report, term.Settings{}, false); err != nil {
		return internalError(err)
	}

	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_CREATE|os.O_WRONLY, outputFileMode)
	if err != nil {
		return fmt.Errorf("write GitHub job summary %s: %w", path, err)
	}

	if _, err := file.Write(buffer.Bytes()); err != nil {
		_ = file.Close()

		return fmt.Errorf("write GitHub job summary %s: %w", path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("write GitHub job summary %s: %w", path, err)
	}

	return nil
}

func inGitHubActions(env term.Env) bool {
	return env[githubActionsEnv] == "true"
}

func renderReport(w io.Writer, format string, report checkReport, settings term.Settings, compact bool) error {
	toolVersion := currentBuildInfo().Version

	switch format {
	case formatJSON:
		return jsonreport.Write(w, jsonreport.Input{
			ToolVersion: toolVersion,
			Dialect:     report.dialect.Name(),
			DBVersion:   report.dbVersion,
			Migrations:  report.migrations,
			Result:      report.result,
			Base:        report.scope.Base,
			ChangedOnly: report.scope.ChangedOnly,
		})
	case formatSARIF:
		return sarif.Write(w, sarif.Input{ToolVersion: toolVersion, Rules: ruleMetas(), Result: report.result})
	case formatGitHub:
		return github.Write(w, github.Input{Result: report.result})
	case formatMarkdown:
		return markdown.Write(w, markdown.Input{
			ToolVersion: toolVersion,
			Dialect:     report.dialect.Name(),
			DBVersion:   report.dbVersion,
			Migrations:  report.migrations,
			Result:      report.result,
			FailOn:      report.failOn,
		})
	case formatJUnit:
		return junit.Write(w, junit.Input{Migrations: report.migrations, Result: report.result, FailOn: report.failOn})
	default:
		return renderPretty(w, report, settings, compact)
	}
}

func renderPretty(w io.Writer, report checkReport, settings term.Settings, compact bool) error {
	out := newDisplay(w, settings)
	options := pretty.Options{
		Theme:   out.theme,
		Width:   out.caps.ContentWidth(),
		Compact: compact || out.caps.Compact(),
	}

	if out.caps.Hyperlinks {
		options.Link = fileLink
	}

	return pretty.Write(out.out, pretty.Input{
		ToolVersion: currentBuildInfo().Version,
		Dialect:     report.dialect.Name(),
		DBVersion:   report.dbVersion,
		Migrations:  report.migrations,
		Result:      report.result,
		FailOn:      report.failOn,
		Elapsed:     report.elapsed,
		Highlight:   report.dialect.Highlight,
		Base:        report.scope.Base,
		ChangedOnly: report.scope.ChangedOnly,
	}, options)
}
