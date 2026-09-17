package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/bbrainttech/migrail/internal/config"
)

func newSchemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "schema config",
		Short:     "Print the JSON schema for .migrail.yaml",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"config"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "config" {
				return fmt.Errorf("unknown schema %q: use config", args[0])
			}

			if _, err := io.WriteString(cmd.OutOrStdout(), config.Schema); err != nil {
				return internalError(fmt.Errorf("write schema: %w", err))
			}

			return nil
		},
	}
}
