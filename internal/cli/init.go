package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SarthkJadhv/sift/internal/appdata"
	"github.com/SarthkJadhv/sift/internal/project"
	"github.com/SarthkJadhv/sift/internal/storage"
	"github.com/SarthkJadhv/sift/internal/storage/repositories"
	"github.com/spf13/cobra"
)

func newInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize SIFT in the current project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// Initialize or load the project configuration.
			workingDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("get working directory: %w", err)
			}

			cfg, configCreated, err := project.Initialize(workingDir)
			if err != nil {
				return fmt.Errorf("initialize project: %w", err)
			}

			// Resolve and create SIFT application directories.
			paths, err := appdata.Resolve()
			if err != nil {
				return fmt.Errorf("resolve application paths: %w", err)
			}

			if err := paths.Ensure(); err != nil {
				return fmt.Errorf("prepare application directories: %w", err)
			}

			// Open the central SQLite database.
			db, err := storage.OpenSQLite(ctx, paths.Database)
			if err != nil {
				return fmt.Errorf("open SIFT database: %w", err)
			}
			defer db.Close()

			// Apply pending database migrations.
			if err := db.Migrate(ctx); err != nil {
				return fmt.Errorf("run database migrations: %w", err)
			}

			// Initialize the project repository.
			projectRepo, err := repositories.NewProjectRepository(db.DB)
			if err != nil {
				return fmt.Errorf("create project repository: %w", err)
			}

			// Register the project in SQLite.
			record := repositories.Project{
				ID:        cfg.ProjectID,
				Name:      cfg.Name,
				RootPath:  cfg.RootPath,
				CreatedAt: cfg.CreatedAt,
			}

			registeredProject, registered, err := projectRepo.Register(
				ctx,
				record,
			)
			if err != nil {
				return fmt.Errorf("register project: %w", err)
			}

			// Display initialization results.
			out := cmd.OutOrStdout()

			if configCreated {
				fmt.Fprintln(out, "Project configuration created.")
			} else {
				fmt.Fprintln(out, "Project configuration already exists.")
			}

			if registered {
				fmt.Fprintln(out, "Project registered in local database.")
			} else {
				fmt.Fprintln(out, "Project already registered in local database.")
			}

			fmt.Fprintf(out, "\nProject: %s\n", registeredProject.Name)
			fmt.Fprintf(out, "Project ID: %s\n", registeredProject.ID)
			fmt.Fprintf(out, "Root: %s\n", registeredProject.RootPath)
			fmt.Fprintf(
				out,
				"Config: %s\n",
				filepath.Join(cfg.RootPath, ".sift", "config.json"),
			)
			fmt.Fprintf(out, "Database: %s\n", db.Path)

			return nil
		},
	}
}
