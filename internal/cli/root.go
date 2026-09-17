package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/config"
	"github.com/bbrainttech/migrail/internal/ir"
	"github.com/bbrainttech/migrail/internal/ui/components"
	"github.com/bbrainttech/migrail/internal/ui/term"
	"github.com/bbrainttech/migrail/internal/ui/theme"
)

const (
	exitOK          = 0
	exitUsage       = 2
	exitInternal    = 4
	exitInterrupted = 130
)

type exitError struct {
	code   int
	err    error
	silent bool
}

func (e *exitError) Error() string {
	return e.err.Error()
}

func (e *exitError) Unwrap() error {
	return e.err
}

func internalError(err error) error {
	return &exitError{code: exitInternal, err: err}
}

func Execute(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root, ui := newRoot(stdout, stderr)
	root.SetArgs(args)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		return exitInterrupted
	}

	code := exitCode(err)
	writeError(stderr, err, code, ui.settings(ciEnabled(false)))

	return code
}

func exitCode(err error) int {
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}

	return exitUsage
}

func writeError(w io.Writer, err error, code int, settings term.Settings) {
	var exitErr *exitError
	if errors.As(err, &exitErr) && exitErr.silent {
		return
	}

	if settings.Theme == term.ThemeAuto || settings.Theme == "" {
		settings.Theme = term.ThemeDark
	}

	caps := term.Detect(w, os.Environ(), settings)
	t := theme.New(caps.Profile, caps.Dark, caps.Unicode)
	out := &colorprofile.Writer{Forward: w, Profile: caps.Profile}

	var configErr *config.Error
	if errors.As(err, &configErr) {
		writeConfigError(out, t, caps.ContentWidth(), configErr)

		return
	}

	_, _ = fmt.Fprintln(out, t.Error.Render(t.Symbols.Error)+" "+t.Fg.Render(sentence(err.Error())))

	if code == exitUsage {
		_, _ = fmt.Fprintln(out, "  "+t.Muted.Render("Run ")+t.Fg.Render("migrail --help")+t.Muted.Render(" for usage."))
	}
}

func writeConfigError(w io.Writer, t theme.Theme, width int, err *config.Error) {
	path := err.Path
	if cwd, cwdErr := os.Getwd(); cwdErr == nil {
		if relative, relErr := filepath.Rel(cwd, err.Path); relErr == nil {
			path = filepath.ToSlash(relative)
		}
	}

	_, _ = fmt.Fprintln(w, t.Error.Render(t.Symbols.Error)+" "+t.Fg.Render("Invalid config "+path))

	source := components.NewSource(err.Source, nil)
	start, end := source.Lines.LineBounds(err.Line)
	offset := min(start+max(err.Column-1, 0), end)
	tokenEnd := offset

	for tokenEnd < end && err.Source[tokenEnd] != ' ' && err.Source[tokenEnd] != ':' {
		tokenEnd++
	}

	frame := components.Frame{Theme: t, Width: width, Severity: ir.SeverityError, Path: path, Label: err.Message}
	lines := append([]string{""}, frame.Render(source, source.Lines.Span(offset, max(tokenEnd, offset+1)))...)

	if err.Suggestion != "" {
		lines = append(lines, "", "  "+t.Muted.Render("Did you mean ")+t.Accent.Render(err.Suggestion)+t.Muted.Render("?"))
	}

	for _, line := range lines {
		_, _ = fmt.Fprintln(w, line)
	}
}

func sentence(message string) string {
	if message == "" {
		return message
	}

	return strings.ToUpper(message[:1]) + message[1:]
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root, _ := newRoot(stdout, stderr)

	return root
}

func newRoot(stdout, stderr io.Writer) (*cobra.Command, *uiFlags) {
	ui := &uiFlags{color: string(term.ColorAuto), theme: string(term.ThemeAuto)}

	root := &cobra.Command{
		Use:   "migrail",
		Short: "Catch dangerous database migrations before they reach production",
		Long: "migrail reads your migrations the way the database runs them. It reports statements that lock tables, " +
			"rewrite them or break code that's still running, and shows how to fix each one.",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			return ui.validate()
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	ui.register(root)

	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newCheckCommand(ui), newExplainCommand(ui), newRulesCommand(ui), newSchemaCommand(), newVersionCommand(ui))
	installHelp(root, ui)

	return root, ui
}
