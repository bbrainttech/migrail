package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/spf13/cobra"

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
	root := newRootCommand(stdout, stderr)
	root.SetArgs(args)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}

	if errors.Is(ctx.Err(), context.Canceled) {
		return exitInterrupted
	}

	code := exitCode(err)
	writeError(stderr, err, code)

	return code
}

func exitCode(err error) int {
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		return exitErr.code
	}

	return exitUsage
}

func writeError(w io.Writer, err error, code int) {
	var exitErr *exitError
	if errors.As(err, &exitErr) && exitErr.silent {
		return
	}

	caps := term.Detect(w, os.Environ(), term.Settings{Color: term.ColorAuto, Theme: term.ThemeDark})
	t := theme.New(caps.Profile, true, caps.Unicode)
	out := &colorprofile.Writer{Forward: w, Profile: caps.Profile}

	_, _ = fmt.Fprintln(out, t.Error.Render(t.Symbols.Error)+" "+t.Fg.Render(sentence(err.Error())))

	if code == exitUsage {
		_, _ = fmt.Fprintln(out, "  "+t.Muted.Render("Run ")+t.Fg.Render("migrail --help")+t.Muted.Render(" for usage."))
	}
}

func sentence(message string) string {
	if message == "" {
		return message
	}

	return strings.ToUpper(message[:1]) + message[1:]
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	ui := &uiFlags{}

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
	root.AddCommand(newCheckCommand(ui), newExplainCommand(ui), newRulesCommand(ui), newVersionCommand(ui))
	installHelp(root, ui)

	return root
}
