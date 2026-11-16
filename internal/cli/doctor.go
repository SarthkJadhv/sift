package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SarthkJadhv/sift/internal/appdata"
	"github.com/SarthkJadhv/sift/internal/config"
	"github.com/SarthkJadhv/sift/internal/project"
	"github.com/SarthkJadhv/sift/internal/storage"
	"github.com/SarthkJadhv/sift/internal/storage/repositories"
	"github.com/spf13/cobra"
)

type doctorCheck struct {
	Name   string
	Status string
	Detail string
}

const (
	statusPass = "PASS"
	statusFail = "FAIL"
	statusSkip = "SKIP"
)

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check SIFT installation and project health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd)
		},
	}
}

func runDoctor(cmd *cobra.Command) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	var checks []doctorCheck
	failures := 0

	addCheck := func(name, status, detail string) {
		checks = append(checks, doctorCheck{
			Name:   name,
			Status: status,
			Detail: detail,
		})

		if status == statusFail {
			failures++
		}
	}

	fmt.Fprintln(out, "SIFT Doctor")
	fmt.Fprintln(out)

	// 1. Check project configuration.
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	root, rootErr := project.FindRoot(workingDir)

	var cfg config.ProjectConfig
	configLoaded := false
	configDir := ""

	if rootErr != nil {
		addCheck("Project configuration", statusFail, rootErr.Error())
	} else {
		configPath := filepath.Join(root, ".sift", "config.json")
		configDir = filepath.Dir(configPath)

		cfg, err = config.Load(configPath)
		if err != nil {
			addCheck(
				"Project configuration",
				statusFail,
				err.Error(),
			)
		} else {
			configLoaded = true
			addCheck(
				"Project configuration",
				statusPass,
				fmt.Sprintf("Loaded project %q", cfg.Name),
			)
		}
	}

	// 2. Resolve and check application data directories.
	paths, pathsErr := appdata.Resolve()
	dataDirsReady := false

	if pathsErr != nil {
		addCheck(
			"Application data directory",
			statusFail,
			pathsErr.Error(),
		)
	} else {
		var dirErrors []error

		for _, dir := range []string{
			paths.RootDir,
			paths.BackupsDir,
		} {
			info, statErr := os.Stat(dir)
			if statErr != nil {
				dirErrors = append(dirErrors, statErr)
				continue
			}

			if !info.IsDir() {
				dirErrors = append(
					dirErrors,
					fmt.Errorf("%s is not a directory", dir),
				)
			}
		}

		if len(dirErrors) > 0 {
			addCheck(
				"Application data directory",
				statusFail,
				errors.Join(dirErrors...).Error(),
			)
		} else {
			dataDirsReady = true
			addCheck(
				"Application data directory",
				statusPass,
				paths.RootDir,
			)
		}
	}

	// 3. Check SQLite connectivity.
	var db *storage.SQLiteDB
	dbReady := false

	if pathsErr != nil {
		addCheck(
			"Database connectivity",
			statusSkip,
			"Application data path is unavailable",
		)
	} else {
		_, err := os.Stat(paths.Database)

		if errors.Is(err, os.ErrNotExist) {
			addCheck(
				"Database connectivity",
				statusFail,
				"Database does not exist; run sift init",
			)
		} else if err != nil {
			addCheck(
				"Database connectivity",
				statusFail,
				err.Error(),
			)
		} else {
			db, err = storage.OpenSQLite(ctx, paths.Database)

			if err != nil {
				addCheck(
					"Database connectivity",
					statusFail,
					err.Error(),
				)
			} else {
				defer db.Close()

				if err := db.Ping(ctx); err != nil {
					addCheck(
						"Database connectivity",
						statusFail,
						err.Error(),
					)
				} else {
					dbReady = true
					addCheck(
						"Database connectivity",
						statusPass,
						"SQLite connection successful",
					)
				}
			}
		}
	}

	// 4. Check database migration status.
	if !dbReady {
		addCheck(
			"Migration status",
			statusSkip,
			"Database is unavailable",
		)
	} else {
		count, countErr := db.AppliedMigrationCount(ctx)

		if countErr != nil {
			addCheck(
				"Migration status",
				statusFail,
				countErr.Error(),
			)
		} else {
			applied, migrationErr := db.MigrationApplied(ctx, 1)

			if migrationErr != nil {
				addCheck(
					"Migration status",
					statusFail,
					migrationErr.Error(),
				)
			} else if !applied {
				addCheck(
					"Migration status",
					statusFail,
					"Required migration 001 is not applied",
				)
			} else {
				addCheck(
					"Migration status",
					statusPass,
					fmt.Sprintf("%d migration(s) recorded", count),
				)
			}
		}
	}

	// 5. Verify that the current project is registered.
	if !configLoaded || !dbReady {
		addCheck(
			"Project registration",
			statusSkip,
			"Project configuration or database unavailable",
		)
	} else {
		repo, repoErr := repositories.NewProjectRepository(db.DB)

		if repoErr != nil {
			addCheck(
				"Project registration",
				statusFail,
				repoErr.Error(),
			)
		} else {
			registered, lookupErr := repo.GetByID(ctx, cfg.ProjectID)

			if lookupErr != nil {
				addCheck(
					"Project registration",
					statusFail,
					lookupErr.Error(),
				)
			} else if registered.RootPath != cfg.RootPath ||
				registered.Name != cfg.Name {
				addCheck(
					"Project registration",
					statusFail,
					"Database metadata does not match project configuration",
				)
			} else {
				addCheck(
					"Project registration",
					statusPass,
					fmt.Sprintf("Project %q is registered", cfg.Name),
				)
			}
		}
	}

	// 6. Check filesystem permissions.
	if pathsErr != nil {
		addCheck(
			"Filesystem permissions",
			statusSkip,
			"Application data path is unavailable",
		)
	} else {
		var permissionErrors []error

		dirsToCheck := []string{
			paths.RootDir,
			paths.BackupsDir,
		}

		if configLoaded {
			dirsToCheck = append(dirsToCheck, configDir)
		}

		for _, dir := range dirsToCheck {
			if err := checkWritableDirectory(dir); err != nil {
				permissionErrors = append(permissionErrors, err)
			}
		}

		if len(permissionErrors) > 0 {
			addCheck(
				"Filesystem permissions",
				statusFail,
				errors.Join(permissionErrors...).Error(),
			)
		} else if dataDirsReady {
			addCheck(
				"Filesystem permissions",
				statusPass,
				"Required directories are writable",
			)
		} else {
			addCheck(
				"Filesystem permissions",
				statusSkip,
				"Application directories are unavailable",
			)
		}
	}

	// Print diagnostic results.
	for _, check := range checks {
		fmt.Fprintf(
			out,
			"[%s] %-27s %s\n",
			check.Status,
			check.Name,
			check.Detail,
		)
	}

	passed := len(checks) - failures
	fmt.Fprintln(out)
	fmt.Fprintf(
		out,
		"%d checks completed: %d passed, %d failed.\n",
		len(checks),
		passed,
		failures,
	)

	if failures > 0 {
		return fmt.Errorf("SIFT doctor found %d issue(s)", failures)
	}

	fmt.Fprintln(out, "SIFT is healthy.")

	return nil
}

// checkWritableDirectory verifies that a directory exists
// and allows the current user to create temporary files.
func checkWritableDirectory(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%s: %w", dir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	file, err := os.CreateTemp(dir, ".sift-doctor-*")
	if err != nil {
		return fmt.Errorf("%s is not writable: %w", dir, err)
	}

	tempPath := file.Name()

	if err := file.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("close permission probe: %w", err)
	}

	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("remove permission probe: %w", err)
	}

	return nil
}
