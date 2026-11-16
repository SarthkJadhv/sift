package cli

import (
	"errors"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/SarthkJadhv/sift/internal/appdata"
	"github.com/SarthkJadhv/sift/internal/storage"
	"github.com/SarthkJadhv/sift/internal/storage/repositories"
	"github.com/spf13/cobra"
)

func newListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all registered SIFT projects",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Resolve SIFT application data paths.
			paths, err := appdata.Resolve()
			if err != nil {
				return fmt.Errorf("resolve application paths: %w", err)
			}

			// Handle a fresh SIFT installation.
			_, err = os.Stat(paths.Database)

			if errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(
					cmd.OutOrStdout(),
					"No SIFT projects registered.",
				)
				return nil
			}

			if err != nil {
				return fmt.Errorf("inspect database: %w", err)
			}

			// Open the central SQLite database.
			db, err := storage.OpenSQLite(ctx, paths.Database)
			if err != nil {
				return fmt.Errorf("open SIFT database: %w", err)
			}
			defer db.Close()

			// Apply pending migrations.
			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("run database migrations: %w", err)
			}

			// Retrieve registered projects.
			repo, err := repositories.NewProjectRepository(db.DB)
			if err != nil {
				return fmt.Errorf("create project repository: %w", err)
			}

			projects, err := repo.List(ctx)
			if err != nil {
				return fmt.Errorf("list projects: %w", err)
			}

			out := cmd.OutOrStdout()

			if len(projects) == 0 {
				fmt.Fprintln(out, "No SIFT projects registered.")
				return nil
			}

			// Format the output as a table.
			writer := tabwriter.NewWriter(
				out,
				0,
				0,
				2,
				' ',
				0,
			)

			fmt.Fprintln(
				writer,
				"NAME\tPROJECT ID\tROOT PATH",
			)

			for _, p := range projects {
				fmt.Fprintf(
					writer,
					"%s\t%s\t%s\n",
					p.Name,
					p.ID,
					p.RootPath,
				)
			}

			if err := writer.Flush(); err != nil {
				return fmt.Errorf("write project list: %w", err)
			}

			fmt.Fprintf(
				out,
				"\nTotal projects: %d\n",
				len(projects),
			)

			return nil
		},
	}
}
