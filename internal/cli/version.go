package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var version = "dev"

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Display SIFT version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "SIFT version: %s\n", version)
			fmt.Fprintf(cmd.OutOrStdout(), "Go version:   %s\n", runtime.Version())
			fmt.Fprintf(cmd.OutOrStdout(), "OS/Arch:      %s/%s\n", runtime.GOOS, runtime.GOARCH)

			return nil
		},
	}
}
