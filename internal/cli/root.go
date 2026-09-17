package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

const (
	exitOK          = 0
	exitUsage       = 2
	exitInternal    = 4
	exitInterrupted = 130
)

type exitError struct {
	code int
	err  error
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
	_, _ = fmt.Fprintf(w, "migrail: %v\n", err)

	if code == exitUsage {
		_, _ = fmt.Fprintln(w, "Run 'migrail --help' for usage.")
	}
}

func newRootCommand(stdout, stderr io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "migrail",
		Short:         "Catch dangerous database migrations before they reach production",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newVersionCommand())

	return root
}
