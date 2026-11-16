package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SarthkJadhv/sift/internal/project"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize SIFT in the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Get the current working directory.
			workingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			// Initialize the project.
			cfg, created, err := project.Initialize(workingDir)
			if err != nil {
				return fmt.Errorf("initialize project: %w", err)
			}

			configPath := filepath.Join(
				cfg.RootPath,
				".sift",
				"config.json",
			)

			out := cmd.OutOrStdout()

			if created {
				fmt.Fprintln(out, "SIFT project initialized successfully.")
			} else {
				fmt.Fprintln(out, "SIFT project is already initialized.")
			}

			fmt.Fprintf(out, "\nProject: %s\n", cfg.Name)
			fmt.Fprintf(out, "Project ID: %s\n", cfg.ProjectID)
			fmt.Fprintf(out, "Root: %s\n", cfg.RootPath)
			fmt.Fprintf(out, "Config: %s\n", configPath)

			return nil
		},
	}
}
