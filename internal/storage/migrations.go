package storage

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type migration struct {
	Version int
	Name    string
	SQL     string
}

// Migrate applies all unapplied database migrations.
func (s *SQLiteDB) Migrate(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return errors.New("SQLite database is not initialized")
	}

	_, err := s.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create migration tracking table: %w", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	for _, m := range migrations {
		if err := s.applyMigration(ctx, m); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.Name, err)
		}
	}

	return nil
}

// loadMigrations loads migrations from the embedded filesystem.
func loadMigrations() ([]migration, error) {
	return loadMigrationsFrom(migrationFS)
}

// loadMigrationsFrom reads and validates migrations from a filesystem.
// Accepting fs.FS makes the loader independently testable.
func loadMigrationsFrom(source fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(source, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read migration directory: %w", err)
	}

	var migrations []migration
	seenVersions := make(map[int]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		parts := strings.SplitN(name, "_", 2)

		if len(parts) != 2 || parts[1] == ".sql" {
			return nil, fmt.Errorf("invalid migration filename: %s", name)
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil || version <= 0 {
			return nil, fmt.Errorf("invalid migration version: %s", name)
		}

		if seenVersions[version] {
			return nil, fmt.Errorf("duplicate migration version: %d", version)
		}

		seenVersions[version] = true

		filePath := path.Join("migrations", name)

		data, err := fs.ReadFile(source, filePath)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}

		sqlContent := strings.TrimSpace(string(data))

		if sqlContent == "" {
			return nil, fmt.Errorf("migration %s is empty", name)
		}

		migrations = append(migrations, migration{
			Version: version,
			Name:    name,
			SQL:     sqlContent,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// applyMigration executes one migration transactionally.
func (s *SQLiteDB) applyMigration(
	ctx context.Context,
	m migration,
) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer tx.Rollback()

	var exists int

	err = tx.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE version = ?
		)`,
		m.Version,
	).Scan(&exists)

	if err != nil {
		return fmt.Errorf("check migration status: %w", err)
	}

	if exists == 1 {
		return nil
	}

	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return fmt.Errorf("execute migration SQL: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO schema_migrations
			(version, name, applied_at)
			VALUES (?, ?, ?)`,
		m.Version,
		m.Name,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}

	return nil
}

// MigrationApplied checks whether a migration has been applied.
func (s *SQLiteDB) MigrationApplied(
	ctx context.Context,
	version int,
) (bool, error) {
	if s == nil || s.DB == nil {
		return false, errors.New("SQLite database is not initialized")
	}

	var exists int

	err := s.DB.QueryRowContext(
		ctx,
		`SELECT EXISTS (
			SELECT 1 FROM schema_migrations WHERE version = ?
		)`,
		version,
	).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("check migration status: %w", err)
	}

	return exists == 1, nil
}

// AppliedMigrationCount returns the number of applied migrations.
func (s *SQLiteDB) AppliedMigrationCount(
	ctx context.Context,
) (int, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("SQLite database is not initialized")
	}

	var count int

	err := s.DB.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&count)

	if err != nil {
		return 0, fmt.Errorf("count applied migrations: %w", err)
	}

	return count, nil
}
